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
	"errors"
	"fmt"
	"sort"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/prompt"
)

const (
	confirmAzureSubscriptionPromptFmt             = "Use subscription '%v'?"
	selectAzureSubscriptionPrompt                 = "Select a subscription:"
	confirmAzureCreateResourceGroupPrompt         = "Create a new resource group?"
	enterAzureResourceGroupNamePrompt             = "Enter a resource group name"
	enterAzureResourceGroupNamePlaceholder        = "Enter resource group name"
	selectAzureResourceGroupLocationPrompt        = "Select a location for the resource group:"
	selectAzureResourceGroupPrompt                = "Select a resource group:"
	enterAzureServicePrincipalAppIDPrompt         = "Enter the `appId` of the service principal used to create Azure resources"
	enterAzureServicePrincipalAppIDPlaceholder    = "Enter appId..."
	enterAzureServicePrincipalPasswordPrompt      = "Enter the `password` of the service principal used to create Azure resources"
	enterAzureServicePrincipalPasswordPlaceholder = "Enter password..."
	enterAzureServicePrincipalTenantIDPrompt      = "Enter the `tenantId` of the service principal used to create Azure resources"
	enterAzureServicePrincipalTenantIDPlaceholder = "Enter tenantId..."
	azureServicePrincipalCreateInstructionsFmt    = "\nAn Azure service principal with a corresponding role assignment on your resource group is required to create Azure resources.\n\nFor example, you can create one using the following command:\n\033[36maz ad sp create-for-rbac --role Owner --scope /subscriptions/%s/resourceGroups/%s\033[0m\n\nFor more information refer to https://docs.microsoft.com/cli/azure/ad/sp?view=azure-cli-latest#az-ad-sp-create-for-rbac and https://aka.ms/azadsp-more\n\n"
)

func (r *Runner) enterAzureCloudProvider(ctx context.Context, options *initOptions) (*azure.Provider, error) {
	subscription, err := r.selectAzureSubscription(ctx)
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	resourceGroup, err := r.selectAzureResourceGroup(ctx, *subscription)
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	r.Output.LogInfo(azureServicePrincipalCreateInstructionsFmt, subscription.ID, resourceGroup)

	clientID, err := r.Prompter.GetTextInput(enterAzureServicePrincipalAppIDPrompt, prompt.TextInputOptions{
		Placeholder: enterAzureServicePrincipalAppIDPlaceholder,
		Validate:    prompt.ValidateUUIDv4,
	})
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	clientSecret, err := r.Prompter.GetTextInput(enterAzureServicePrincipalPasswordPrompt, prompt.TextInputOptions{Placeholder: enterAzureServicePrincipalPasswordPlaceholder})
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	tenantID, err := r.Prompter.GetTextInput(enterAzureServicePrincipalTenantIDPrompt, prompt.TextInputOptions{
		Placeholder: enterAzureServicePrincipalTenantIDPlaceholder,
		Validate:    prompt.ValidateResourceName,
	})
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
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

func (r *Runner) selectAzureSubscription(ctx context.Context) (*azure.Subscription, error) {
	subscriptions, err := r.azureClient.Subscriptions(ctx)
	if err != nil {
		return nil, &cli.FriendlyError{Message: fmt.Sprintf("failed to list subscriptions: %s", err.Error())}
	}

	// Users can configure a default subscription with `az account set`. If they did, then ask about that first.
	if subscriptions.Default != nil {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(confirmAzureSubscriptionPromptFmt, subscriptions.Default.Name), prompt.ConfirmYes, r.Prompter)
		if errors.Is(err, &prompt.ErrExitConsole{}) {
			return nil, &cli.FriendlyError{Message: err.Error()}
		} else if err != nil {
			return nil, err
		}

		if confirmed {
			return subscriptions.Default, nil
		}
	}

	names, subscriptionMap := r.buildAzureSubscriptionListAndMap(subscriptions)
	name, err := r.Prompter.GetListInput(names, selectAzureSubscriptionPrompt)
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	subscription := subscriptionMap[name]
	return &subscription, nil
}

// buildSubscriptionListAndMap builds a list of subscription names, as well as a map of name => subcription. We need the list
// to build the prompt, and the map to look up the subscription object by name after the user makes a selection.
func (r *Runner) buildAzureSubscriptionListAndMap(subscriptions *azure.SubscriptionResult) ([]string, map[string]azure.Subscription) {
	subscriptionMap := map[string]azure.Subscription{}
	names := []string{}
	for _, s := range subscriptions.Subscriptions {
		subscriptionMap[s.Name] = s
		names = append(names, s.Name)
	}

	sort.Strings(names)

	return names, subscriptionMap
}

func (r *Runner) selectAzureResourceGroup(ctx context.Context, subscription azure.Subscription) (string, error) {
	create, err := prompt.YesOrNoPrompt(confirmAzureCreateResourceGroupPrompt, prompt.ConfirmYes, r.Prompter)
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return "", &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return "", err
	}

	if !create {
		return r.selectExistingAzureResourceGroup(ctx, subscription)
	}

	name, err := r.enterAzureResourceGroupName(ctx)
	if err != nil {
		return "", err
	}

	exists, err := r.azureClient.CheckResourceGroupExistence(ctx, subscription.ID, name)
	if err != nil {
		return "", err
	}

	// Nothing left to do.
	if exists {
		return name, nil
	}

	r.Output.LogInfo("Resource Group '%v' will be created...", name)

	location, err := r.selectAzureResourceGroupLocation(ctx, subscription)
	if err != nil {
		return "", err
	}

	err = r.azureClient.CreateOrUpdateResourceGroup(ctx, subscription.ID, name, *location.Name)
	if err != nil {
		return "", &cli.FriendlyError{Message: fmt.Sprintf("Failed to create Azure resource group: %v", err.Error())}
	}

	return name, nil
}

func (r *Runner) selectExistingAzureResourceGroup(ctx context.Context, subscription azure.Subscription) (string, error) {
	groups, err := r.azureClient.ResourceGroups(ctx, subscription.ID)
	if err != nil {
		return "", &cli.FriendlyError{Message: fmt.Sprintf("Failed to get list Azure resource groups: %v", err.Error())}
	}

	names := r.buildAzureResourceGroupList(groups)
	name, err := r.Prompter.GetListInput(names, selectAzureResourceGroupPrompt)
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return "", &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return "", err
	}

	return name, nil
}

func (r *Runner) buildAzureResourceGroupList(groups []armresources.ResourceGroup) []string {
	names := []string{}
	for _, s := range groups {
		names = append(names, *s.Name)
	}

	sort.Strings(names)

	return names
}

func (r *Runner) enterAzureResourceGroupName(ctx context.Context) (string, error) {
	name, err := r.Prompter.GetTextInput(enterAzureResourceGroupNamePrompt, prompt.TextInputOptions{
		Placeholder: enterAzureResourceGroupNamePlaceholder,
		Validate:    prompt.ValidateResourceName,
	})
	if errors.Is(err, &prompt.ErrExitConsole{}) {
		return "", &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return "", err
	}

	return name, nil
}

func (r *Runner) selectAzureResourceGroupLocation(ctx context.Context, subscription azure.Subscription) (*armsubscriptions.Location, error) {
	// Use the display name for the prompt
	// alphabetize so the list is stable and scannable
	locations, err := r.azureClient.Locations(ctx, subscription.ID)
	if err != nil {
		return nil, &cli.FriendlyError{Message: fmt.Sprintf("Failed to get list Azure locations: %v", err.Error())}
	}

	names, locationMap := r.buildAzureResourceGroupLocationListAndMap(locations)
	name, err := r.Prompter.GetListInput(names, selectAzureResourceGroupLocationPrompt)
	if err != nil {
		return nil, err
	}

	location := locationMap[name]
	return &location, nil
}

// buildAzureResourceGroupLocationListAndMap builds a list of location names, as well as a map of name => location. We need the list
// to build the prompt, and the map to look up the location object by name after the user makes a selection.
func (r *Runner) buildAzureResourceGroupLocationListAndMap(locations []armsubscriptions.Location) ([]string, map[string]armsubscriptions.Location) {
	locationMap := map[string]armsubscriptions.Location{}
	names := []string{}
	for _, location := range locations {
		names = append(names, *location.DisplayName)
		locationMap[*location.DisplayName] = location
	}

	sort.Strings(names)

	return names, locationMap
}
