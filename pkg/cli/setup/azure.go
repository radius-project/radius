// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/marstr/randname"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"

	"github.com/project-radius/radius/pkg/azure/armauth"
	cli_aws "github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/to"
)

// RegisterAzureProviderArgs adds flags to configure Azure provider for cloud resources.
func RegisterPersistentAzureProviderArgs(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP("provider-azure", "", false, "Add Azure provider for cloud resources")
	cmd.PersistentFlags().String("provider-azure-subscription", "", "Azure subscription for cloud resources")
	cmd.PersistentFlags().String("provider-azure-resource-group", "", "Azure resource-group for cloud resources")
	cmd.PersistentFlags().StringP("provider-azure-client-id", "", "", "The client id for the service principal")
	cmd.PersistentFlags().StringP("provider-azure-client-secret", "", "", "The client secret for the service principal")
	cmd.PersistentFlags().StringP("provider-azure-tenant-id", "", "", "The tenant id for the service principal")
}

func ParseAzureProviderArgs(cmd *cobra.Command, interactive bool, prompter prompt.Interface) (*azure.Provider, error) {
	if interactive {
		return parseAzureProviderInteractive(cmd, prompter)
	}
	return parseAzureProviderNonInteractive(cmd)
}

func parseAzureProviderInteractive(cmd *cobra.Command, prompter prompt.Interface) (*azure.Provider, error) {
	armConfig, err := armauth.NewArmConfig(nil)
	if err != nil {
		return nil, err
	}

	subscription, err := selectSubscription(cmd.Context(), prompter, armConfig)
	if err != nil {
		return nil, err
	}
	resourceGroup, err := selectResourceGroup(cmd.Context(), subscription, prompter, armConfig)
	if err != nil {
		return nil, err
	}

	fmt.Printf(
		"\nAn Azure service principal with a corresponding role assignment on your resource group is required to create Azure resources.\n\nFor example, you can create one using the following command:\n\033[36maz ad sp create-for-rbac --role Owner --scope /subscriptions/%s/resourceGroups/%s\033[0m\n\nFor more information refer to https://docs.microsoft.com/cli/azure/ad/sp?view=azure-cli-latest#az-ad-sp-create-for-rbac and https://aka.ms/azadsp-more\n\n",
		subscription.ID,
		resourceGroup,
	)

	clientID, err := prompter.GetTextInput(
		"Enter the `appId` of the service principal used to create Azure resources",
		"Enter AppId...",
	)
	if err != nil {
		return nil, err
	}
	isValid, errMsg, _ := prompt.UUIDv4Validator(clientID)
	if !isValid {
		return nil, fmt.Errorf(errMsg)
	}

	clientSecret, err := prompter.GetTextInput(
		"Enter the `password` of the service principal used to create Azure resources",
		"",
	)
	if err != nil {
		return nil, err
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("Client Secret cannot be Empty")
	}

	tenantID, err := prompter.GetTextInput(
		"Enter the `tenant` of the service principal used to create Azure resources",
		"",
	)
	if err != nil {
		return nil, err
	}
	isValid, errMsg, _ = prompt.UUIDv4Validator(tenantID)
	if !isValid {
		return nil, fmt.Errorf(errMsg)
	}

	return &azure.Provider{
		SubscriptionID: subscription.ID,
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

	armConfig, err := armauth.NewArmConfig(nil)
	if err != nil {
		return nil, err
	}

	if isValid, _, _ := prompt.UUIDv4Validator(subscriptionID); !isValid {
		// NOTE: These functions interact with the Azure CLI. We only want to do this
		// if the user opts in to doing azure.
		//
		// At this point we've already exited the function unless the user set `--provider-azure`
		// so it should be safe to interact with the Azure CLI.
		subs, err := azure.LoadSubscriptionsFromProfile()
		if err != nil {
			// Failed to load subscriptions from the user profile, fall back to online.
			subs, err = azure.LoadSubscriptionsFromAzure(cmd.Context(), armConfig.ClientOptions)
			if err != nil {
				return nil, err
			}
		}
		idx := slices.IndexFunc(subs.Subscriptions, func(c azure.Subscription) bool { return c.Name == subscriptionID })
		if idx != -1 {
			subscriptionID = subs.Subscriptions[idx].ID
		} else {
			return nil, fmt.Errorf("valid --provider-azure-subscription is required to configure Azure provider for cloud resources")
		}
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

func selectSubscription(ctx context.Context, prompter prompt.Interface, armConfig *armauth.ArmConfig) (azure.Subscription, error) {
	subs, err := azure.LoadSubscriptionsFromProfile()
	if err != nil {
		// Failed to load subscriptions from the user profile, fall back to online.
		subs, err = azure.LoadSubscriptionsFromAzure(ctx, armConfig.ClientOptions)
		if err != nil {
			return azure.Subscription{}, err
		}
	}

	if subs.Default != nil {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf("Use Subscription '%v'?", subs.Default.Name), "yes", prompter)
		if err != nil {
			return azure.Subscription{}, err
		}

		if confirmed {
			return *subs.Default, nil
		}
	}

	// build prompt to select from list
	sort.Slice(subs.Subscriptions, func(i, j int) bool {
		l := strings.ToLower(subs.Subscriptions[i].Name)
		r := strings.ToLower(subs.Subscriptions[j].Name)
		return l < r
	})
	subscriptionMap := make(map[string]azure.Subscription)
	names := make([]string, 0, len(subs.Subscriptions))
	for _, s := range subs.Subscriptions {
		subscriptionMap[s.Name] = s
		names = append(names, s.Name)
	}

	name, err := prompter.GetListInput(names, "Select Subscription")
	if err != nil {
		return azure.Subscription{}, err
	}

	return subscriptionMap[name], nil
}

func selectResourceGroup(ctx context.Context, sub azure.Subscription, prompter prompt.Interface, armConfig *armauth.ArmConfig) (string, error) {
	client, err := armresources.NewResourceGroupsClient(sub.ID, armConfig.ClientOptions.Cred, nil)
	if err != nil {
		return "", err
	}

	name, err := promptUserForRgName(ctx, client, prompter)
	if err != nil {
		return "", err
	}

	resp, err := client.CheckExistence(ctx, name, &armresources.ResourceGroupsClientCheckExistenceOptions{})
	if err != nil {
		return "", err
	}

	if resp.Success {
		// Resource Group already exists
		return name, nil
	}

	output.LogInfo("Resource Group '%v' will be created...", name)

	location, err := promptUserForLocation(ctx, sub, prompter, armConfig)
	if err != nil {
		return "", err
	}

	_, err = client.CreateOrUpdate(ctx, name, armresources.ResourceGroup{
		Location: to.Ptr(*location.Name),
	}, &armresources.ResourceGroupsClientCreateOrUpdateOptions{})
	if err != nil {
		return "", err
	}

	return name, nil
}

func promptUserForLocation(ctx context.Context, sub azure.Subscription, prompter prompt.Interface, armConfig *armauth.ArmConfig) (*armsubscriptions.Location, error) {
	// Use the display name for the prompt
	// alphabetize so the list is stable and scannable
	client, err := armsubscriptions.NewClient(armConfig.ClientOptions.Cred, &arm.ClientOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot list locations: %w", err)
	}

	pager := client.NewListLocationsPager(sub.ID, &armsubscriptions.ClientListLocationsOptions{})
	locations := map[string]*armsubscriptions.Location{}
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		locationList := nextPage.LocationListResult.Value
		for _, loc := range locationList {
			locations[*loc.DisplayName] = loc
		}
	}

	names := []string{}
	for _, loc := range locations {
		names = append(names, *loc.DisplayName)
	}
	sort.Strings(names)

	name, err := prompter.GetListInput(names, fmt.Sprintf("Select a location, Default: %s", names[0]))
	if err != nil {
		return nil, err
	}

	return locations[name], nil
}

func promptUserForRgName(ctx context.Context, client *armresources.ResourceGroupsClient, prompter prompt.Interface) (string, error) {
	var name string
	createNewRg, err := prompt.YesOrNoPrompt("Create a new Resource Group? [Y/n]", "Y", prompter)
	if err != nil {
		return "", err
	}
	if createNewRg {

		defaultRgName := "radius-rg"
		if resp, err := client.CheckExistence(ctx, defaultRgName, nil); !resp.Success || err != nil {
			// only generate a random name if the default doesn't exist already or existence check fails
			defaultRgName = generateRandomName("radius", "rg")
		}

		promptStr := fmt.Sprintf("Enter a Resource Group name [%s]:", defaultRgName)

		name, err = prompter.GetTextInput(promptStr, defaultRgName)
		if err != nil {
			return "", err
		}
		if name == "" {
			return "", fmt.Errorf("Resource group cannot be empty")
		}
	} else {
		requestFilter := url.QueryEscape("")
		pager := client.NewListPager(&armresources.ResourceGroupsClientListOptions{
			Filter: &requestFilter,
		})
		resourceGroups := []armresources.ResourceGroup{}
		for pager.More() {
			nextPage, err := pager.NextPage(ctx)
			if err != nil {
				return "", err
			}

			list := nextPage.ResourceGroupListResult.Value
			for _, rg := range list {
				resourceGroups = append(resourceGroups, *rg)
			}
		}

		sort.Slice(resourceGroups, func(i, j int) bool {
			return strings.ToLower(*resourceGroups[i].Name) < strings.ToLower(*resourceGroups[j].Name)
		})

		names := []string{}
		for _, s := range resourceGroups {
			names = append(names, *s.Name)
		}

		name, err = prompter.GetListInput(names, "Select ResourceGroup")
		if err != nil {
			return "", err
		}
	}

	return name, nil
}

// GenerateRandomName generates a string with the specified prefix and a random 5-character suffix.
func generateRandomName(prefix string, affixes ...string) string {
	b := bytes.NewBufferString(prefix)
	b.WriteRune('-')
	for _, affix := range affixes {
		b.WriteString(affix)
		b.WriteRune('-')
	}
	return randname.GenerateWithPrefix(b.String(), 5)
}

//go:generate mockgen -destination=./mock_setup.go -package=setup -self_package github.com/project-radius/radius/pkg/cli/setup github.com/project-radius/radius/pkg/cli/setup Interface
type Interface interface {
	ParseAzureProviderArgs(cmd *cobra.Command, interactive bool, prompter prompt.Interface) (*azure.Provider, error)
	ParseAWSProviderArgs(cmd *cobra.Command, interactive bool, prompter prompt.Interface) (*cli_aws.Provider, error)
}

type Impl struct {
}

// Parses user input from the CLI for Azure Provider arguments
func (i *Impl) ParseAzureProviderArgs(cmd *cobra.Command, interactive bool, prompter prompt.Interface) (*azure.Provider, error) {
	return ParseAzureProviderArgs(cmd, interactive, prompter)
}

// Parses user input from the CLI for Azure Provider arguments
func (i *Impl) ParseAWSProviderArgs(cmd *cobra.Command, interactive bool, prompter prompt.Interface) (*cli_aws.Provider, error) {
	return ParseAWSProviderArgs(cmd, interactive, prompter)
}
