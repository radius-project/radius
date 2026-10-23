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

package radinit

import (
	"context"
	"fmt"
	"sort"

	"charm.land/bubbles/v2/textinput"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

const (
	confirmAzureSubscriptionPromptFmt             = "Use subscription '%v'?"
	selectAzureSubscriptionPrompt                 = "Select a subscription:"
	confirmAzureCreateResourceGroupPrompt         = "Create a new resource group?"
	enterAzureResourceGroupNamePrompt             = "Enter a resource group name"
	enterAzureResourceGroupNamePlaceholder        = "Enter resource group name"
	selectAzureResourceGroupLocationPrompt        = "Select a location for the resource group:"
	selectAzureResourceGroupPrompt                = "Select a resource group:"
	selectAzureCredentialKindPrompt               = "Select a credential kind for the Azure credential:"
	enterAzureServicePrincipalAppIDPrompt         = "Enter the `appId` of the service principal used to create Azure resources"
	enterAzureServicePrincipalAppIDPlaceholder    = "Enter appId..."
	enterAzureServicePrincipalPasswordPrompt      = "Enter the `password` of the service principal used to create Azure resources"
	enterAzureServicePrincipalPasswordPlaceholder = "Enter password..."
	enterAzureServicePrincipalTenantIDPrompt      = "Enter the `tenantId` of the service principal used to create Azure resources"
	enterAzureServicePrincipalTenantIDPlaceholder = "Enter tenantId..."
	enterAzureWorkloadIdentityAppIDPrompt         = "Enter the `appId` of the Entra ID Application"
	enterAzureWorkloadIdentityAppIDPlaceholder    = "Enter appId..."
	enterAzureWorkloadIdentityTenantIDPrompt      = "Enter the `tenantId` of the Entra ID Application"
	enterAzureWorkloadIdentityTenantIDPlaceholder = "Enter tenantId..."
	azureWorkloadIdentityCreateInstructionsFmt    = "\nA workload identity federated credential is required to create Azure resources. Please follow the guidance at aka.ms/rad-workload-identity to set up workload identity for Radius.\n\n"
	azureServicePrincipalCreateInstructionsFmt    = "\nAn Azure service principal with a corresponding role assignment on your resource group is required to create Azure resources.\n\nFor example, you can create one using the following command:\n\033[36maz ad sp create-for-rbac --role Owner --scope /subscriptions/%s/resourceGroups/%s\033[0m\n\nFor more information refer to https://docs.microsoft.com/cli/azure/ad/sp?view=azure-cli-latest#az-ad-sp-create-for-rbac and https://aka.ms/azadsp-more\n\n"
	azureServicePrincipalCredentialKind           = "Service Principal"
	azureWorkloadIdenityCredentialKind            = "Workload Identity"
)

// enterAzureCloudProvider prompts the user for Azure cloud provider configuration. When
// workload identity is selected it also enables workload identity in the Helm values.
func (r *Runner) enterAzureCloudProvider(ctx context.Context, options *initOptions) (*azure.Provider, error) {
	subscription, err := selectAzureSubscription(ctx, r.Prompter, r.azureClient)
	if err != nil {
		return nil, err
	}

	resourceGroup, err := selectAzureResourceGroup(ctx, r.Prompter, r.Output, r.azureClient, *subscription)
	if err != nil {
		return nil, err
	}

	credentialKind, err := selectAzureCredentialKind(r.Prompter)
	if err != nil {
		return nil, err
	}

	switch credentialKind {
	case azureServicePrincipalCredentialKind:
		r.Output.LogInfo(azureServicePrincipalCreateInstructionsFmt, subscription.ID, resourceGroup)

		clientID, err := r.Prompter.GetTextInput(enterAzureServicePrincipalAppIDPrompt, prompt.TextInputOptions{
			Placeholder: enterAzureServicePrincipalAppIDPlaceholder,
			Validate:    prompt.ValidateUUIDv4,
		})
		if err != nil {
			return nil, err
		}

		clientSecret, err := r.Prompter.GetTextInput(enterAzureServicePrincipalPasswordPrompt, prompt.TextInputOptions{Placeholder: enterAzureServicePrincipalPasswordPlaceholder, EchoMode: textinput.EchoPassword})
		if err != nil {
			return nil, err
		}

		tenantID, err := r.Prompter.GetTextInput(enterAzureServicePrincipalTenantIDPrompt, prompt.TextInputOptions{
			Placeholder: enterAzureServicePrincipalTenantIDPlaceholder,
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
	case azureWorkloadIdenityCredentialKind:
		r.Output.LogInfo(azureWorkloadIdentityCreateInstructionsFmt)

		clientID, err := r.Prompter.GetTextInput(enterAzureWorkloadIdentityAppIDPrompt, prompt.TextInputOptions{
			Placeholder: enterAzureWorkloadIdentityAppIDPlaceholder,
			Validate:    prompt.ValidateUUIDv4,
		})
		if err != nil {
			return nil, err
		}

		tenantID, err := r.Prompter.GetTextInput(enterAzureWorkloadIdentityTenantIDPrompt, prompt.TextInputOptions{
			Placeholder: enterAzureWorkloadIdentityTenantIDPlaceholder,
			Validate:    prompt.ValidateUUIDv4,
		})
		if err != nil {
			return nil, err
		}

		// Set the value for the Helm chart.
		options.SetValues = append(options.SetValues, "global.azureWorkloadIdentity.enabled=true")

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

// selectAzureSubscription prompts the user to select an Azure subscription. If
// a default subscription is configured the user is asked whether to use it.
func selectAzureSubscription(ctx context.Context, prompter prompt.Interface, azureClient azure.Client) (*azure.Subscription, error) {
	subscriptions, err := azureClient.Subscriptions(ctx)
	if err != nil {
		return nil, clierrors.MessageWithCause(err, "Failed to list Azure subscriptions.")
	}

	// Users can configure a default subscription with `az account set`. If they did, then ask about that first.
	if subscriptions.Default != nil {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(confirmAzureSubscriptionPromptFmt, subscriptions.Default.Name), prompt.ConfirmYes, prompter)
		if err != nil {
			return nil, err
		}

		if confirmed {
			return subscriptions.Default, nil
		}
	}

	names, subscriptionMap := buildAzureSubscriptionListAndMap(subscriptions)
	name, err := prompter.GetListInput(names, selectAzureSubscriptionPrompt)
	if err != nil {
		return nil, err
	}

	subscription := subscriptionMap[name]
	return &subscription, nil
}

// selectAzureCredentialKind prompts the user to select an Azure credential kind.
func selectAzureCredentialKind(prompter prompt.Interface) (string, error) {
	return prompter.GetListInput(buildAzureCredentialKindList(), selectAzureCredentialKindPrompt)
}

// buildAzureSubscriptionListAndMap builds a list of subscription names and a
// map of name => subscription for use by the prompt.
func buildAzureSubscriptionListAndMap(subscriptions *azure.SubscriptionResult) ([]string, map[string]azure.Subscription) {
	subscriptionMap := map[string]azure.Subscription{}
	names := []string{}
	for _, s := range subscriptions.Subscriptions {
		subscriptionMap[s.Name] = s
		names = append(names, s.Name)
	}

	sort.Strings(names)

	return names, subscriptionMap
}

// selectAzureResourceGroup either creates a new resource group or prompts the
// user to choose an existing one.
func selectAzureResourceGroup(ctx context.Context, prompter prompt.Interface, out output.Interface, azureClient azure.Client, subscription azure.Subscription) (string, error) {
	create, err := prompt.YesOrNoPrompt(confirmAzureCreateResourceGroupPrompt, prompt.ConfirmYes, prompter)
	if err != nil {
		return "", err
	}

	if !create {
		return selectExistingAzureResourceGroup(ctx, prompter, azureClient, subscription)
	}

	name, err := enterAzureResourceGroupName(prompter)
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

	location, err := selectAzureResourceGroupLocation(ctx, prompter, azureClient, subscription)
	if err != nil {
		return "", err
	}

	err = azureClient.CreateOrUpdateResourceGroup(ctx, subscription.ID, name, *location.Name)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Failed to create Azure resource group.")
	}

	return name, nil
}

// selectExistingAzureResourceGroup prompts the user to pick from existing
// resource groups in the given subscription.
func selectExistingAzureResourceGroup(ctx context.Context, prompter prompt.Interface, azureClient azure.Client, subscription azure.Subscription) (string, error) {
	groups, err := azureClient.ResourceGroups(ctx, subscription.ID)
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Failed to get list Azure resource groups.")
	}

	names := buildAzureResourceGroupList(groups)
	name, err := prompter.GetListInput(names, selectAzureResourceGroupPrompt)
	if err != nil {
		return "", err
	}

	return name, nil
}

// buildAzureResourceGroupList builds a sorted list of resource group names.
func buildAzureResourceGroupList(groups []armresources.ResourceGroup) []string {
	names := []string{}
	for _, s := range groups {
		names = append(names, *s.Name)
	}

	sort.Strings(names)

	return names
}

// enterAzureResourceGroupName prompts the user for a resource group name.
func enterAzureResourceGroupName(prompter prompt.Interface) (string, error) {
	return prompter.GetTextInput(enterAzureResourceGroupNamePrompt, prompt.TextInputOptions{
		Placeholder: enterAzureResourceGroupNamePlaceholder,
		Validate:    prompt.ValidateResourceName,
	})
}

// selectAzureResourceGroupLocation prompts the user to pick a location for a
// new resource group.
func selectAzureResourceGroupLocation(ctx context.Context, prompter prompt.Interface, azureClient azure.Client, subscription azure.Subscription) (*armsubscriptions.Location, error) {
	locations, err := azureClient.Locations(ctx, subscription.ID)
	if err != nil {
		return nil, clierrors.MessageWithCause(err, "Failed to get list Azure locations.")
	}

	names, locationMap := buildAzureResourceGroupLocationListAndMap(locations)
	name, err := prompter.GetListInput(names, selectAzureResourceGroupLocationPrompt)
	if err != nil {
		return nil, err
	}

	location := locationMap[name]
	return &location, nil
}

// buildAzureResourceGroupLocationListAndMap builds a sorted list of location
// display names and a map of display name => location.
func buildAzureResourceGroupLocationListAndMap(locations []armsubscriptions.Location) ([]string, map[string]armsubscriptions.Location) {
	locationMap := map[string]armsubscriptions.Location{}
	names := []string{}
	for _, location := range locations {
		names = append(names, *location.DisplayName)
		locationMap[*location.DisplayName] = location
	}

	sort.Strings(names)

	return names, locationMap
}

// buildAzureCredentialKindList returns the list of supported Azure credential kinds.
func buildAzureCredentialKindList() []string {
	return []string{
		azureServicePrincipalCredentialKind,
		azureWorkloadIdenityCredentialKind,
	}
}
