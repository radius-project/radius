/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"context"
	"fmt"
	"sort"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

const (
	ConfirmAzureSubscriptionPromptFmt             = "Use subscription '%v'?"
	SelectAzureSubscriptionPrompt                 = "Select a subscription:"
	ConfirmAzureCreateResourceGroupPrompt         = "Create a new resource group?"
	EnterAzureResourceGroupNamePrompt             = "Enter a resource group name"
	EnterAzureResourceGroupNamePlaceholder        = "Enter resource group name"
	SelectAzureResourceGroupLocationPrompt        = "Select a location for the resource group:"
	SelectAzureResourceGroupPrompt                = "Select a resource group:"
	SelectAzureCredentialKindPrompt               = "Select a credential kind for the Azure credential:"
	EnterAzureServicePrincipalAppIDPrompt         = "Enter the `appId` of the service principal used to create Azure resources"
	EnterAzureServicePrincipalAppIDPlaceholder    = "Enter appId..."
	EnterAzureServicePrincipalPasswordPrompt      = "Enter the `password` of the service principal used to create Azure resources"
	EnterAzureServicePrincipalPasswordPlaceholder = "Enter password..."
	EnterAzureServicePrincipalTenantIDPrompt      = "Enter the `tenantId` of the service principal used to create Azure resources"
	EnterAzureServicePrincipalTenantIDPlaceholder = "Enter tenantId..."
	EnterAzureWorkloadIdentityAppIDPrompt         = "Enter the `appId` of the Entra ID Application"
	EnterAzureWorkloadIdentityAppIDPlaceholder    = "Enter appId..."
	EnterAzureWorkloadIdentityTenantIDPrompt      = "Enter the `tenantId` of the Entra ID Application"
	EnterAzureWorkloadIdentityTenantIDPlaceholder = "Enter tenantId..."
	AzureWorkloadIdentityCreateInstructionsFmt    = "\nA workload identity federated credential is required to create Azure resources. Please follow the guidance at aka.ms/rad-workload-identity to set up workload identity for Radius.\n\n"
	AzureServicePrincipalCreateInstructionsFmt    = "\nAn Azure service principal with a corresponding role assignment on your resource group is required to create Azure resources.\n\nFor example, you can create one using the following command:\n\033[36maz ad sp create-for-rbac --role Owner --scope /subscriptions/%s/resourceGroups/%s\033[0m\n\nFor more information refer to https://docs.microsoft.com/cli/azure/ad/sp?view=azure-cli-latest#az-ad-sp-create-for-rbac and https://aka.ms/azadsp-more\n\n"
	AzureServicePrincipalCredentialKind           = "Service Principal"
	AzureWorkloadIdenityCredentialKind            = "Workload Identity"
)

// EnterAzureCloudProvider prompts the user for Azure cloud provider configuration.
// The caller is responsible for any post-processing such as enabling workload
// identity Helm values based on the returned provider's CredentialKind.
func EnterAzureCloudProvider(ctx context.Context, prompter prompt.Interface, out output.Interface, azureClient azure.Client) (*azure.Provider, error) {
	subscription, err := SelectAzureSubscription(ctx, prompter, azureClient)
	if err != nil {
		return nil, err
	}

	resourceGroup, err := SelectAzureResourceGroup(ctx, prompter, out, azureClient, *subscription)
	if err != nil {
		return nil, err
	}

	credentialKind, err := SelectAzureCredentialKind(prompter)
	if err != nil {
		return nil, err
	}

	switch credentialKind {
	case AzureServicePrincipalCredentialKind:
		out.LogInfo(AzureServicePrincipalCreateInstructionsFmt, subscription.ID, resourceGroup)

		clientID, err := prompter.GetTextInput(EnterAzureServicePrincipalAppIDPrompt, prompt.TextInputOptions{
			Placeholder: EnterAzureServicePrincipalAppIDPlaceholder,
			Validate:    prompt.ValidateUUIDv4,
		})
		if err != nil {
			return nil, err
		}

		clientSecret, err := prompter.GetTextInput(EnterAzureServicePrincipalPasswordPrompt, prompt.TextInputOptions{Placeholder: EnterAzureServicePrincipalPasswordPlaceholder, EchoMode: textinput.EchoPassword})
		if err != nil {
			return nil, err
		}

		tenantID, err := prompter.GetTextInput(EnterAzureServicePrincipalTenantIDPrompt, prompt.TextInputOptions{
			Placeholder: EnterAzureServicePrincipalTenantIDPlaceholder,
			Validate:    prompt.ValidateUUIDv4,
		})
		if err != nil {
			return nil, err
		}

		return &azure.Provider{
			SubscriptionID: subscription.ID,
			ResourceGroup:  resourceGroup,
			CredentialKind: azure.AzureCredentialKindServicePrincipal,
			ServicePrincipal: &azure.ServicePrincipalCredential{
				ClientID:     clientID,
				ClientSecret: clientSecret,
				TenantID:     tenantID,
			},
		}, nil
	case AzureWorkloadIdenityCredentialKind:
		out.LogInfo(AzureWorkloadIdentityCreateInstructionsFmt)

		clientID, err := prompter.GetTextInput(EnterAzureWorkloadIdentityAppIDPrompt, prompt.TextInputOptions{
			Placeholder: EnterAzureWorkloadIdentityAppIDPlaceholder,
			Validate:    prompt.ValidateUUIDv4,
		})
		if err != nil {
			return nil, err
		}

		tenantID, err := prompter.GetTextInput(EnterAzureWorkloadIdentityTenantIDPrompt, prompt.TextInputOptions{
			Placeholder: EnterAzureWorkloadIdentityTenantIDPlaceholder,
			Validate:    prompt.ValidateUUIDv4,
		})
		if err != nil {
			return nil, err
		}

		return &azure.Provider{
			SubscriptionID: subscription.ID,
			ResourceGroup:  resourceGroup,
			CredentialKind: azure.AzureCredentialKindWorkloadIdentity,
			WorkloadIdentity: &azure.WorkloadIdentityCredential{
				ClientID: clientID,
				TenantID: tenantID,
			},
		}, nil
	default:
		return nil, clierrors.Message("Invalid Azure credential kind: %s", credentialKind)
	}
}

// SelectAzureSubscription prompts the user to select an Azure subscription. If
// a default subscription is configured the user is asked whether to use it.
func SelectAzureSubscription(ctx context.Context, prompter prompt.Interface, azureClient azure.Client) (*azure.Subscription, error) {
	subscriptions, err := azureClient.Subscriptions(ctx)
	if err != nil {
		return nil, clierrors.MessageWithCause(err, "Failed to list Azure subscriptions.")
	}

	// Users can configure a default subscription with `az account set`. If they did, then ask about that first.
	if subscriptions.Default != nil {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(ConfirmAzureSubscriptionPromptFmt, subscriptions.Default.Name), prompt.ConfirmYes, prompter)
		if err != nil {
			return nil, err
		}

		if confirmed {
			return subscriptions.Default, nil
		}
	}

	names, subscriptionMap := BuildAzureSubscriptionListAndMap(subscriptions)
	name, err := prompter.GetListInput(names, SelectAzureSubscriptionPrompt)
	if err != nil {
		return nil, err
	}

	subscription := subscriptionMap[name]
	return &subscription, nil
}

// SelectAzureCredentialKind prompts the user to select an Azure credential kind.
func SelectAzureCredentialKind(prompter prompt.Interface) (string, error) {
	return prompter.GetListInput(BuildAzureCredentialKindList(), SelectAzureCredentialKindPrompt)
}

// BuildAzureSubscriptionListAndMap builds a list of subscription names and a
// map of name => subscription for use by the prompt.
func BuildAzureSubscriptionListAndMap(subscriptions *azure.SubscriptionResult) ([]string, map[string]azure.Subscription) {
	subscriptionMap := map[string]azure.Subscription{}
	names := []string{}
	for _, s := range subscriptions.Subscriptions {
		subscriptionMap[s.Name] = s
		names = append(names, s.Name)
	}

	sort.Strings(names)

	return names, subscriptionMap
}

// SelectAzureResourceGroup either creates a new resource group or prompts the
// user to choose an existing one.
func SelectAzureResourceGroup(ctx context.Context, prompter prompt.Interface, out output.Interface, azureClient azure.Client, subscription azure.Subscription) (string, error) {
	create, err := prompt.YesOrNoPrompt(ConfirmAzureCreateResourceGroupPrompt, prompt.ConfirmYes, prompter)
	if err != nil {
		return "", err
	}

	if !create {
		return SelectExistingAzureResourceGroup(ctx, prompter, azureClient, subscription)
	}

	name, err := EnterAzureResourceGroupName(prompter)
	if err != nil {
		return "", err
	}

	exists, err := azureClient.CheckResourceGroupExistence(ctx, subscription.ID, name)
	if err != nil {
		return "", err
	}

	// Nothing left to do.
	if exists {
		return name, nil
	}

	out.LogInfo("Resource Group '%v' will be created...", name)

	location, err := SelectAzureResourceGroupLocation(ctx, prompter, azureClient, subscription)
	if err != nil {
		return "", err
	}

	err = azureClient.CreateOrUpdateResourceGroup(ctx, subscription.ID, name, *location.Name)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Failed to create Azure resource group.")
	}

	return name, nil
}

// SelectExistingAzureResourceGroup prompts the user to pick from existing
// resource groups in the given subscription.
func SelectExistingAzureResourceGroup(ctx context.Context, prompter prompt.Interface, azureClient azure.Client, subscription azure.Subscription) (string, error) {
	groups, err := azureClient.ResourceGroups(ctx, subscription.ID)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Failed to get list Azure resource groups.")
	}

	names := BuildAzureResourceGroupList(groups)
	name, err := prompter.GetListInput(names, SelectAzureResourceGroupPrompt)
	if err != nil {
		return "", err
	}

	return name, nil
}

// BuildAzureResourceGroupList builds a sorted list of resource group names.
func BuildAzureResourceGroupList(groups []armresources.ResourceGroup) []string {
	names := []string{}
	for _, s := range groups {
		names = append(names, *s.Name)
	}

	sort.Strings(names)

	return names
}

// EnterAzureResourceGroupName prompts the user for a resource group name.
func EnterAzureResourceGroupName(prompter prompt.Interface) (string, error) {
	return prompter.GetTextInput(EnterAzureResourceGroupNamePrompt, prompt.TextInputOptions{
		Placeholder: EnterAzureResourceGroupNamePlaceholder,
		Validate:    prompt.ValidateResourceName,
	})
}

// SelectAzureResourceGroupLocation prompts the user to pick a location for a
// new resource group.
func SelectAzureResourceGroupLocation(ctx context.Context, prompter prompt.Interface, azureClient azure.Client, subscription azure.Subscription) (*armsubscriptions.Location, error) {
	locations, err := azureClient.Locations(ctx, subscription.ID)
	if err != nil {
		return nil, clierrors.MessageWithCause(err, "Failed to get list Azure locations.")
	}

	names, locationMap := BuildAzureResourceGroupLocationListAndMap(locations)
	name, err := prompter.GetListInput(names, SelectAzureResourceGroupLocationPrompt)
	if err != nil {
		return nil, err
	}

	location := locationMap[name]
	return &location, nil
}

// BuildAzureResourceGroupLocationListAndMap builds a sorted list of location
// display names and a map of display name => location.
func BuildAzureResourceGroupLocationListAndMap(locations []armsubscriptions.Location) ([]string, map[string]armsubscriptions.Location) {
	locationMap := map[string]armsubscriptions.Location{}
	names := []string{}
	for _, location := range locations {
		names = append(names, *location.DisplayName)
		locationMap[*location.DisplayName] = location
	}

	sort.Strings(names)

	return names, locationMap
}

// BuildAzureCredentialKindList returns the list of supported Azure credential kinds.
func BuildAzureCredentialKindList() []string {
	return []string{
		AzureServicePrincipalCredentialKind,
		AzureWorkloadIdenityCredentialKind,
	}
}
