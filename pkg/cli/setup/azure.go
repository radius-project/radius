// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/spf13/cobra"
)

// RegisterAzureProviderArgs adds flags to configure Azure provider for cloud resources.
func RegistePersistantAzureProviderArgs(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP("provider-azure", "", false, "Add Azure provider for cloud resources")
	cmd.PersistentFlags().String("provider-azure-subscription", "", "Azure subscription for cloud resources")
	cmd.PersistentFlags().String("provider-azure-resource-group", "", "Azure resource-group for cloud resources")
	cmd.PersistentFlags().StringP("provider-azure-client-id", "", "", "The client id for the service principal")
	cmd.PersistentFlags().StringP("provider-azure-client-secret", "", "", "The client secret for the service principal")
	cmd.PersistentFlags().StringP("provider-azure-tenant-id", "", "", "The tenant id for the service principal")
}

func ParseAzureProviderArgs(cmd *cobra.Command, interactive bool) (*azure.Provider, error) {
	if interactive {
		return parseAzureProviderInteractive(cmd)
	}
	return parseAzureProviderNonInteractive(cmd)
}

func parseAzureProviderInteractive(cmd *cobra.Command) (*azure.Provider, error) {
	authorizer, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return nil, err
	}

	addAzureSPN, err := prompt.ConfirmWithDefault("Add Azure provider for cloud resources [y/N]?", prompt.No)
	if err != nil {
		return nil, err
	}
	if !addAzureSPN {
		return nil, nil
	}

	subscription, err := selectSubscription(cmd.Context(), authorizer)
	if err != nil {
		return nil, err
	}
	resourceGroup, err := selectResourceGroup(cmd.Context(), authorizer, subscription)
	if err != nil {
		return nil, err
	}

	fmt.Printf(
		"\nA Service Principal Name (SPN) with a corresponding role assignment and scope for your resource group is required to create Azure resources.\n\nFor example, you can create one using the following command:\n\033[36maz ad sp create-for-rbac --role Owner --scope /subscriptions/%s/resourceGroups/%s\033[0m\n\nFor more information, see: https://docs.microsoft.com/cli/azure/ad/sp?view=azure-cli-latest#az-ad-sp-create-for-rbac and https://aka.ms/azadsp-more\n\n",
		subscription.SubscriptionID,
		resourceGroup,
	)

	clientID, err := prompt.Text(
		"Enter the `appId` of the service principal used to create Azure resources:",
		prompt.UUIDv4Validator,
	)
	if err != nil {
		return nil, err
	}

	clientSecret, err := prompt.Text(
		"Enter the `password` of the service principal used to create Azure resources:",
		prompt.EmptyValidator,
	)
	if err != nil {
		return nil, err
	}

	tenantID, err := prompt.Text(
		"Enter the `tenant` of the service principal used to create Azure resources:",
		prompt.UUIDv4Validator,
	)
	if err != nil {
		return nil, err
	}

	return &azure.Provider{
		SubscriptionID: subscription.SubscriptionID,
		ResourceGroup:  resourceGroup,
		ServicePrincipal: &azure.ServicePrincipal{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TenantID:     tenantID,
		},
	}, nil
}

func parseAzureProviderNonInteractive(cmd *cobra.Command) (*azure.Provider, error) {
	subscriptionID, err := cmd.Flags().GetString("provider-azure-subscription")
	if err != nil {
		return nil, err
	}
	resourceGroup, err := cmd.Flags().GetString("provider-azure-resource-group")
	if err != nil {
		return nil, err
	}

	addAzureSPN, err := cmd.Flags().GetBool("provider-azure")
	if err != nil {
		return nil, err
	}
	if !addAzureSPN {
		if subscriptionID == "" && resourceGroup == "" {
			return nil, nil
		}
		return &azure.Provider{
			SubscriptionID: subscriptionID,
			ResourceGroup:  resourceGroup,
		}, nil
	}
	clientID, err := cmd.Flags().GetString("provider-azure-client-id")
	if err != nil {
		return nil, err
	}
	clientSecret, err := cmd.Flags().GetString("provider-azure-client-secret")
	if err != nil {
		return nil, err
	}
	tenantID, err := cmd.Flags().GetString("provider-azure-tenant-id")
	if err != nil {
		return nil, err
	}
	if isValid, _, _ := prompt.UUIDv4Validator(subscriptionID); !isValid {
		return nil, fmt.Errorf("--provider-azure-subscription is required to configure Azure provider for cloud resources")
	}
	if resourceGroup == "" {
		return nil, fmt.Errorf("--provider-azure-resource-group is required to configure Azure provider for cloud resources")
	}
	if isValid, _, _ := prompt.UUIDv4Validator(clientID); !isValid {
		return nil, errors.New("--provider-azure-client-id parameter is required to configure Azure provider for cloud resources")
	}
	if clientSecret == "" {
		return nil, errors.New("--provider-azure-client-secret parameter is required to configure Azure provider for cloud resources")
	}
	if isValid, _, _ := prompt.UUIDv4Validator(tenantID); !isValid {
		return nil, errors.New("--provider-azure-tenant-id parameter is required to configure Azure provider for cloud resources")
	}
	if (subscriptionID != "") != (resourceGroup != "") {
		return nil, fmt.Errorf("to use the Azure provider both --provider-azure-subscription and --provider-azure-resource-group must be provided")
	}

	return &azure.Provider{
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
		ServicePrincipal: &azure.ServicePrincipal{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TenantID:     tenantID,
		},
	}, nil
}

func selectSubscription(ctx context.Context, authorizer autorest.Authorizer) (azure.Subscription, error) {
	subs, err := azure.LoadSubscriptionsFromProfile()
	if err != nil {
		// Failed to load subscriptions from the user profile, fall back to online.
		subs, err = azure.LoadSubscriptionsFromAzure(ctx, authorizer)
		if err != nil {
			return azure.Subscription{}, err
		}
	}

	if subs.Default != nil {
		confirmed, err := prompt.ConfirmWithDefault(fmt.Sprintf("Use Subscription '%v'? [Y/n]", subs.Default.DisplayName), prompt.Yes)
		if err != nil {
			return azure.Subscription{}, err
		}

		if confirmed {
			return *subs.Default, nil
		}
	}

	// build prompt to select from list
	sort.Slice(subs.Subscriptions, func(i, j int) bool {
		l := strings.ToLower(subs.Subscriptions[i].DisplayName)
		r := strings.ToLower(subs.Subscriptions[j].DisplayName)
		return l < r
	})
	names := make([]string, 0, len(subs.Subscriptions))
	for _, s := range subs.Subscriptions {
		names = append(names, s.DisplayName)
	}

	index, err := prompt.SelectWithDefault("Select Subscription:", &names[0], names)
	if err != nil {
		return azure.Subscription{}, err
	}

	return subs.Subscriptions[index], nil
}

func selectResourceGroup(ctx context.Context, authorizer autorest.Authorizer, sub azure.Subscription) (string, error) {
	rgc := clients.NewGroupsClient(sub.SubscriptionID, authorizer)
	name, err := promptUserForRgName(ctx, rgc)
	if err != nil {
		return "", err
	}
	resp, err := rgc.CheckExistence(ctx, name)
	if err != nil {
		return "", err
	} else if !resp.HasHTTPStatus(404) {
		// already exists
		return name, nil
	}
	output.LogInfo("Resource Group '%v' will be created...", name)

	location, err := promptUserForLocation(ctx, authorizer, sub)
	if err != nil {
		return "", err
	}
	_, err = rgc.CreateOrUpdate(ctx, name, resources.Group{
		Location: to.StringPtr(*location.Name),
	})
	if err != nil {
		return "", err
	}

	return name, nil
}

func promptUserForLocation(ctx context.Context, authorizer autorest.Authorizer, sub azure.Subscription) (subscription.Location, error) {
	// Use the display name for the prompt
	// alphabetize so the list is stable and scannable
	subc := clients.NewSubscriptionClient(authorizer)

	locations, err := subc.ListLocations(ctx, sub.SubscriptionID)
	if err != nil {
		return subscription.Location{}, fmt.Errorf("cannot list locations: %w", err)
	}

	names := make([]string, 0, len(*locations.Value))
	nameToLocation := map[string]subscription.Location{}
	for _, loc := range *locations.Value {

		names = append(names, *loc.DisplayName)
		nameToLocation[*loc.DisplayName] = loc
	}
	sort.Strings(names)

	index, err := prompt.SelectWithDefault("Select a location:", &names[0], names)
	if err != nil {
		return subscription.Location{}, err
	}
	selected := names[index]
	return nameToLocation[selected], nil
}

func promptUserForRgName(ctx context.Context, rgc resources.GroupsClient) (string, error) {
	var name string
	createNewRg, err := prompt.ConfirmWithDefault("Create a new Resource Group? [Y/n]", prompt.Yes)
	if err != nil {
		return "", err
	}
	if createNewRg {

		defaultRgName := "radius-rg"
		if resp, err := rgc.CheckExistence(ctx, defaultRgName); !resp.HasHTTPStatus(404) || err != nil {
			// only generate a random name if the default doesn't exist already or existence check fails
			defaultRgName = handlers.GenerateRandomName("radius", "rg")
		}

		promptStr := fmt.Sprintf("Enter a Resource Group name [%s]:", defaultRgName)
		name, err = prompt.TextWithDefault(promptStr, &defaultRgName, prompt.EmptyValidator)
		if err != nil {
			return "", err
		}
	} else {
		rgListResp, err := rgc.List(ctx, "", nil)
		if err != nil {
			return "", err
		}
		rgList := rgListResp.Values()
		sort.Slice(rgList, func(i, j int) bool { return strings.ToLower(*rgList[i].Name) < strings.ToLower(*rgList[j].Name) })
		names := make([]string, 0, len(rgList))
		for _, s := range rgList {
			names = append(names, *s.Name)
		}

		defaultRgName, _ := azure.LoadDefaultResourceGroupFromConfig() // ignore errors resulting from being unable to read the config ini file
		index, err := prompt.SelectWithDefault("Select ResourceGroup:", &defaultRgName, names)
		if err != nil {
			return "", err
		}
		name = *rgList[index].Name
	}
	return name, nil
}
