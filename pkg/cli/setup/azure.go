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
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/preview/preview/subscription/mgmt/subscription"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/marstr/randname"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"

	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
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
	addAzureSPN, err := prompt.YesOrNoPrompter("Add Azure provider for cloud resources [y/N]?", "N", prompter)
	if err != nil {
		return nil, err
	}
	if strings.ToLower(addAzureSPN) == "n" {
		return nil, nil
	}

	// NOTE: These functions interact with the Azure CLI. We only want to do this
	// if the user opts in to doing azure.
	//
	// At this point we've already asked the user so we should be ok.
	authorizer, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return nil, err
	}

	subscription, err := selectSubscription(cmd.Context(), authorizer, prompter)
	if err != nil {
		return nil, err
	}
	resourceGroup, err := selectResourceGroup(cmd.Context(), authorizer, subscription, prompter)
	if err != nil {
		return nil, err
	}

	clientID, err := prompter.RunPrompt(prompt.TextPromptWithDefault(
		"Enter the `appId` of the service principal used to create Azure resources:",
		"",
		prompt.UUIDv4Validator,
	))
	if err != nil {
		return nil, err
	}

	clientSecret, err := prompter.RunPrompt(prompt.TextPromptWithDefault(
		"Enter the `password` of the service principal used to create Azure resources:",
		"",
		prompt.EmptyValidator,
	))
	if err != nil {
		return nil, err
	}

	tenantID, err := prompter.RunPrompt(prompt.TextPromptWithDefault(
		"Enter the `tenant` of the service principal used to create Azure resources:",
		"",
		prompt.UUIDv4Validator,
	))
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
		// NOTE: These functions interact with the Azure CLI. We only want to do this
		// if the user opts in to doing azure.
		//
		// At this point we've already exited the function unless the user set `--provider-azure`
		// so it should be safe to interact with the Azure CLI.
		subs, err := azure.LoadSubscriptionsFromProfile()
		if err != nil {
			authorizer, err := auth.NewAuthorizerFromCLI()
			if err != nil {
				return nil, err
			}

			// Failed to load subscriptions from the user profile, fall back to online.
			subs, err = azure.LoadSubscriptionsFromAzure(cmd.Context(), authorizer)
			if err != nil {
				return nil, err
			}
		}
		idx := slices.IndexFunc(subs.Subscriptions, func(c azure.Subscription) bool { return c.DisplayName == subscriptionID })
		if idx != -1 {
			subscriptionID = subs.Subscriptions[idx].SubscriptionID
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

func selectSubscription(ctx context.Context, authorizer autorest.Authorizer, prompter prompt.Interface) (azure.Subscription, error) {
	subs, err := azure.LoadSubscriptionsFromProfile()
	if err != nil {
		// Failed to load subscriptions from the user profile, fall back to online.
		subs, err = azure.LoadSubscriptionsFromAzure(ctx, authorizer)
		if err != nil {
			return azure.Subscription{}, err
		}
	}

	if subs.Default != nil {
		confirmed, err := prompt.YesOrNoPrompter(fmt.Sprintf("Use Subscription '%v'? [Y/n]", subs.Default.DisplayName), "Y", prompter)
		if err != nil {
			return azure.Subscription{}, err
		}

		if strings.ToLower(confirmed) == "y" {
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

	index, _, err := prompter.RunSelect(prompt.SelectionPrompter("Select Subscription:", names))
	if err != nil {
		return azure.Subscription{}, err
	}

	return subs.Subscriptions[index], nil
}

func selectResourceGroup(ctx context.Context, authorizer autorest.Authorizer, sub azure.Subscription, prompter prompt.Interface) (string, error) {
	rgc := clients.NewGroupsClient(sub.SubscriptionID, authorizer)
	name, err := promptUserForRgName(ctx, rgc, prompter)
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

	location, err := promptUserForLocation(ctx, authorizer, sub, prompter)
	if err != nil {
		return "", err
	}
	_, err = rgc.CreateOrUpdate(ctx, name, resources.Group{
		Location: to.Ptr(*location.Name),
	})
	if err != nil {
		return "", err
	}

	return name, nil
}

func promptUserForLocation(ctx context.Context, authorizer autorest.Authorizer, sub azure.Subscription, prompter prompt.Interface) (subscription.Location, error) {
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
	index, _, err := prompter.RunSelect(prompt.SelectionPrompter(fmt.Sprintf("Select a location, Default: %s", names[0]), names))
	if err != nil {
		return subscription.Location{}, err
	}
	selected := names[index]
	return nameToLocation[selected], nil
}

func promptUserForRgName(ctx context.Context, rgc resources.GroupsClient, prompter prompt.Interface) (string, error) {
	var name string
	createNewRg, err := prompt.YesOrNoPrompter("Create a new Resource Group? [Y/n]", "Y", prompter)
	if err != nil {
		return "", err
	}
	if strings.ToLower(createNewRg) == "y" {

		defaultRgName := "radius-rg"
		if resp, err := rgc.CheckExistence(ctx, defaultRgName); !resp.HasHTTPStatus(404) || err != nil {
			// only generate a random name if the default doesn't exist already or existence check fails
			defaultRgName = generateRandomName("radius", "rg")
		}

		promptStr := fmt.Sprintf("Enter a Resource Group name [%s]:", defaultRgName)

		name, err = prompter.RunPrompt(prompt.TextPromptWithDefault(promptStr, defaultRgName, prompt.EmptyValidator))
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
		yes, err := prompt.YesOrNoPrompter(fmt.Sprintf("Use default resource group %s [Y/n]", defaultRgName), "Y", prompter)
		if err != nil {
			return "", err
		}
		if strings.ToLower(yes) == "y" {
			return defaultRgName, nil
		}
		index, _, err := prompter.RunSelect(prompt.SelectionPrompter("Select ResourceGroup", names))
		if err != nil {
			return "", err
		}
		name = *rgList[index].Name
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
}

type Impl struct {
}

// Parses user input from the CLI for Azure Provider arguments
func (i *Impl) ParseAzureProviderArgs(cmd *cobra.Command, interactive bool, prompter prompt.Interface) (*azure.Provider, error) {
	return ParseAzureProviderArgs(cmd, interactive, prompter)
}
