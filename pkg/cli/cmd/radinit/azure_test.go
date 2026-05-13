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
	"sort"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/cmd/radinit/common"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_enterAzureCloudProvider_ServicePrincipal(t *testing.T) {
	ctrl := gomock.NewController(t)
	prompter := prompt.NewMockInterface(ctrl)
	client := azure.NewMockClient(ctrl)
	outputSink := output.MockOutput{}
	runner := Runner{Prompter: prompter, azureClient: client, Output: &outputSink}

	subscription := azure.Subscription{
		Name: "test-subscription",
		ID:   "test-subscription-id",
	}

	resourceGroup := armresources.ResourceGroup{
		Name: new("test-resource-group"),
	}

	setAzureSubscriptions(client, &azure.SubscriptionResult{Default: &subscription, Subscriptions: []azure.Subscription{subscription}})
	setAzureSubscriptionConfirmPrompt(prompter, subscription.Name, prompt.ConfirmYes)

	setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmNo)
	setAzureResourceGroups(client, subscription.ID, []armresources.ResourceGroup{resourceGroup})
	setAzureResourceGroupPrompt(prompter, []string{*resourceGroup.Name}, *resourceGroup.Name)

	setAzureCredentialKindPrompt(prompter, "Service Principal")

	setAzureServicePrincipalAppIDPrompt(prompter, "service-principal-app-id")
	setAzureServicePrincipalPasswordPrompt(prompter, "service-principal-password")
	setAzureServicePrincipalTenantIDPrompt(prompter, "service-principal-tenant-id")

	options := &initOptions{}

	provider, err := runner.enterAzureCloudProvider(context.Background(), options)
	require.NoError(t, err)

	expected := &azure.Provider{
		SubscriptionID: subscription.ID,
		ResourceGroup:  *resourceGroup.Name,
		CredentialKind: "ServicePrincipal",
		ServicePrincipal: &azure.ServicePrincipalCredential{
			ClientID:     "service-principal-app-id",
			ClientSecret: "service-principal-password",
			TenantID:     "service-principal-tenant-id",
		},
	}
	require.Equal(t, expected, provider)

	expectedOutput := []any{output.LogOutput{
		Format: common.AzureServicePrincipalCreateInstructionsFmt,
		Params: []any{subscription.ID, *resourceGroup.Name},
	}}
	require.Equal(t, expectedOutput, outputSink.Writes)

	expectedOptions := &initOptions{}
	require.Equal(t, expectedOptions, options)
}

func Test_enterAzureCloudProvider_WorkloadIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	prompter := prompt.NewMockInterface(ctrl)
	client := azure.NewMockClient(ctrl)
	outputSink := output.MockOutput{}
	runner := Runner{Prompter: prompter, azureClient: client, Output: &outputSink}

	subscription := azure.Subscription{
		Name: "test-subscription",
		ID:   "test-subscription-id",
	}

	resourceGroup := armresources.ResourceGroup{
		Name: new("test-resource-group"),
	}

	setAzureSubscriptions(client, &azure.SubscriptionResult{Default: &subscription, Subscriptions: []azure.Subscription{subscription}})
	setAzureSubscriptionConfirmPrompt(prompter, subscription.Name, prompt.ConfirmYes)

	setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmNo)
	setAzureResourceGroups(client, subscription.ID, []armresources.ResourceGroup{resourceGroup})
	setAzureResourceGroupPrompt(prompter, []string{*resourceGroup.Name}, *resourceGroup.Name)

	setAzureCredentialKindPrompt(prompter, "Workload Identity")

	setAzureWorkloadIdentityAppIDPrompt(prompter, "service-principal-app-id")
	setAzureWorkloadIdentityTenantIDPrompt(prompter, "service-principal-tenant-id")

	options := &initOptions{}

	provider, err := runner.enterAzureCloudProvider(context.Background(), options)
	require.NoError(t, err)

	expected := &azure.Provider{
		SubscriptionID: subscription.ID,
		ResourceGroup:  *resourceGroup.Name,
		CredentialKind: "WorkloadIdentity",
		WorkloadIdentity: &azure.WorkloadIdentityCredential{
			ClientID: "service-principal-app-id",
			TenantID: "service-principal-tenant-id",
		},
	}
	require.Equal(t, expected, provider)

	expectedOutput := []any{output.LogOutput{
		Format: common.AzureWorkloadIdentityCreateInstructionsFmt,
	}}
	require.Equal(t, expectedOutput, outputSink.Writes)

	expectedOptions := &initOptions{}
	expectedOptions.SetValues = []string{"global.azureWorkloadIdentity.enabled=true"}
	require.Equal(t, expectedOptions, options)
}

func Test_selectAzureSubscription(t *testing.T) {
	// Intentionally not in sorted order
	subscriptions := azure.SubscriptionResult{
		Subscriptions: []azure.Subscription{
			{
				Name: "b-test-subscription1",
				ID:   "test-subscription-id1",
			},
			{
				Name: "c-test-subscription2",
				ID:   "test-subscription-id2",
			},
			{
				Name: "a-test-subscription2",
				ID:   "test-subscription-id2",
			},
		},
	}

	subscriptionNames := []string{}
	for _, subscription := range subscriptions.Subscriptions {
		subscriptionNames = append(subscriptionNames, subscription.Name)
	}
	sort.Strings(subscriptionNames)

	t.Run("choose default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		client := azure.NewMockClient(ctrl)

		subscriptions := subscriptions
		subscriptions.Default = &subscriptions.Subscriptions[1]

		setAzureSubscriptions(client, &subscriptions)
		setAzureSubscriptionConfirmPrompt(prompter, subscriptions.Default.Name, prompt.ConfirmYes)

		selected, err := common.SelectAzureSubscription(context.Background(), prompter, client)
		require.NoError(t, err)
		require.NotNil(t, selected)

		require.Equal(t, *subscriptions.Default, *selected)
	})

	t.Run("choose non-default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		client := azure.NewMockClient(ctrl)

		subscriptions := subscriptions
		subscriptions.Default = &subscriptions.Subscriptions[1]

		setAzureSubscriptions(client, &subscriptions)
		setAzureSubscriptionConfirmPrompt(prompter, subscriptions.Default.Name, prompt.ConfirmNo)
		setAzureSubsubscriptionPrompt(prompter, subscriptionNames, subscriptions.Subscriptions[2].Name)

		selected, err := common.SelectAzureSubscription(context.Background(), prompter, client)
		require.NoError(t, err)
		require.NotNil(t, selected)

		require.Equal(t, subscriptions.Subscriptions[2], *selected)
	})

	t.Run("no-default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		client := azure.NewMockClient(ctrl)

		subscriptions := subscriptions
		subscriptions.Default = nil

		setAzureSubscriptions(client, &subscriptions)
		setAzureSubsubscriptionPrompt(prompter, subscriptionNames, subscriptions.Subscriptions[2].Name)

		selected, err := common.SelectAzureSubscription(context.Background(), prompter, client)
		require.NoError(t, err)
		require.NotNil(t, selected)

		require.Equal(t, subscriptions.Subscriptions[2], *selected)
	})
}

func Test_buildAzureSubscriptionListAndMap(t *testing.T) {
	// Intentionally not in sorted order
	subscriptions := azure.SubscriptionResult{
		Subscriptions: []azure.Subscription{
			{
				Name: "b-test-subscription1",
				ID:   "test-subscription-id1",
			},
			{
				Name: "c-test-subscription2",
				ID:   "test-subscription-id2",
			},
			{
				Name: "a-test-subscription2",
				ID:   "test-subscription-id2",
			},
		},
	}

	expectedNames := []string{"a-test-subscription2", "b-test-subscription1", "c-test-subscription2"}
	expectedMap := map[string]azure.Subscription{
		"a-test-subscription2": subscriptions.Subscriptions[2],
		"b-test-subscription1": subscriptions.Subscriptions[0],
		"c-test-subscription2": subscriptions.Subscriptions[1],
	}

	names, subscriptionMap := common.BuildAzureSubscriptionListAndMap(&subscriptions)
	require.Equal(t, expectedNames, names)
	require.Equal(t, expectedMap, subscriptionMap)
}

func Test_selectAzureResourceGroup(t *testing.T) {
	subscription := azure.Subscription{
		Name: "test-subscription",
		ID:   "test-subscription-id",
	}

	// Intentionally not in alphabetical order
	resourceGroups := []armresources.ResourceGroup{
		{
			Name: new("b-test-resource-group1"),
		},
		{
			Name: new("a-test-resource-group2"),
		},
		{
			Name: new("c-test-resource-group3"),
		},
	}

	resourceGroupNames := []string{}
	for _, resourceGroup := range resourceGroups {
		resourceGroupNames = append(resourceGroupNames, *resourceGroup.Name)
	}
	sort.Strings(resourceGroupNames)

	// Intentionally not in alphabetical order
	locations := []armsubscriptions.Location{
		{
			Name:        new("westus"),
			DisplayName: new("West US"),
		},
		{
			Name:        new("eastus"),
			DisplayName: new("East US"),
		},
	}

	locationDisplayNames := []string{}
	for _, location := range locations {
		locationDisplayNames = append(locationDisplayNames, *location.DisplayName)
	}
	sort.Strings(locationDisplayNames)

	t.Run("choose existing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		client := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}

		setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmNo)
		setAzureResourceGroups(client, subscription.ID, resourceGroups)
		setAzureResourceGroupPrompt(prompter, resourceGroupNames, *resourceGroups[1].Name)

		name, err := common.SelectAzureResourceGroup(context.Background(), prompter, &outputSink, client, subscription)
		require.NoError(t, err)

		require.Equal(t, *resourceGroups[1].Name, name)
		require.Empty(t, outputSink.Writes)
	})

	t.Run("create new", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		client := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}

		setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmYes)
		setAzureResourceGroupNamePrompt(prompter, "test-resource-group")
		setAzureCheckResourceGroupExistence(client, subscription.ID, "test-resource-group", false)
		setAzureLocations(client, subscription.ID, locations)
		setSelectAzureResourceGroupLocationPrompt(prompter, locationDisplayNames, *locations[1].DisplayName)
		setAzureCreateOrUpdateResourceGroup(client, subscription.ID, "test-resource-group", *locations[1].Name)

		name, err := common.SelectAzureResourceGroup(context.Background(), prompter, &outputSink, client, subscription)
		require.NoError(t, err)

		require.Equal(t, "test-resource-group", name)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Resource Group '%v' will be created...",
				Params: []any{"test-resource-group"},
			},
		}
		require.Equal(t, expectedWrites, outputSink.Writes)
	})

	t.Run("create new (exists)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		client := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}

		setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmYes)
		setAzureResourceGroupNamePrompt(prompter, "test-resource-group")
		setAzureCheckResourceGroupExistence(client, subscription.ID, "test-resource-group", true)

		name, err := common.SelectAzureResourceGroup(context.Background(), prompter, &outputSink, client, subscription)
		require.NoError(t, err)

		require.Equal(t, "test-resource-group", name)

		require.Empty(t, outputSink.Writes)
	})
}

func Test_selectExistingAzureResourceGroup(t *testing.T) {
	subscription := azure.Subscription{
		Name: "test-subscription",
		ID:   "test-subscription-id",
	}

	// Intentionally not in alphabetical order
	resourceGroups := []armresources.ResourceGroup{
		{
			Name: new("b-test-resource-group1"),
		},
		{
			Name: new("a-test-resource-group2"),
		},
		{
			Name: new("c-test-resource-group3"),
		},
	}

	resourceGroupNames := []string{}
	for _, resourceGroup := range resourceGroups {
		resourceGroupNames = append(resourceGroupNames, *resourceGroup.Name)
	}
	sort.Strings(resourceGroupNames)

	ctrl := gomock.NewController(t)
	prompter := prompt.NewMockInterface(ctrl)
	client := azure.NewMockClient(ctrl)

	setAzureResourceGroups(client, subscription.ID, resourceGroups)
	setAzureResourceGroupPrompt(prompter, resourceGroupNames, *resourceGroups[1].Name)

	name, err := common.SelectExistingAzureResourceGroup(context.Background(), prompter, client, subscription)
	require.NoError(t, err)

	require.Equal(t, *resourceGroups[1].Name, name)
}

func Test_buildAzureResourceGroupList(t *testing.T) {
	// Intentionally not in alphabetical order
	resourceGroups := []armresources.ResourceGroup{
		{
			Name: new("b-test-resource-group1"),
		},
		{
			Name: new("a-test-resource-group2"),
		},
		{
			Name: new("c-test-resource-group3"),
		},
	}

	expectedNames := []string{"a-test-resource-group2", "b-test-resource-group1", "c-test-resource-group3"}

	names := common.BuildAzureResourceGroupList(resourceGroups)
	require.Equal(t, expectedNames, names)
}

func Test_enterAzureResourceGroupName(t *testing.T) {
	ctrl := gomock.NewController(t)
	prompter := prompt.NewMockInterface(ctrl)

	setAzureResourceGroupNamePrompt(prompter, "test-resource-group")

	name, err := common.EnterAzureResourceGroupName(prompter)
	require.NoError(t, err)
	require.Equal(t, "test-resource-group", name)
}

func Test_selectAzureResourceGroupLocation(t *testing.T) {
	subscription := azure.Subscription{
		Name: "test-subscription",
		ID:   "test-subscription-id",
	}

	// Intentionally not in alphabetical order
	resourceGroups := []armresources.ResourceGroup{
		{
			Name: new("b-test-resource-group1"),
		},
		{
			Name: new("a-test-resource-group2"),
		},
		{
			Name: new("c-test-resource-group3"),
		},
	}

	resourceGroupNames := []string{}
	for _, resourceGroup := range resourceGroups {
		resourceGroupNames = append(resourceGroupNames, *resourceGroup.Name)
	}
	sort.Strings(resourceGroupNames)

	// Intentionally not in alphabetical order
	locations := []armsubscriptions.Location{
		{
			Name:        new("westus"),
			DisplayName: new("West US"),
		},
		{
			Name:        new("eastus"),
			DisplayName: new("East US"),
		},
	}

	locationDisplayNames := []string{}
	for _, location := range locations {
		locationDisplayNames = append(locationDisplayNames, *location.DisplayName)
	}
	sort.Strings(locationDisplayNames)

	ctrl := gomock.NewController(t)
	prompter := prompt.NewMockInterface(ctrl)
	client := azure.NewMockClient(ctrl)

	setAzureLocations(client, subscription.ID, locations)
	setSelectAzureResourceGroupLocationPrompt(prompter, locationDisplayNames, *locations[1].DisplayName)

	location, err := common.SelectAzureResourceGroupLocation(context.Background(), prompter, client, subscription)
	require.NoError(t, err)
	require.Equal(t, locations[1], *location)
}

func Test_buildAzureResourceGroupLocationListAndMap(t *testing.T) {
	// Intentionally not in alphabetical order
	locations := []armsubscriptions.Location{
		{
			Name:        new("westus"),
			DisplayName: new("West US"),
		},
		{
			Name:        new("eastus"),
			DisplayName: new("East US"),
		},
	}

	expectedNames := []string{"East US", "West US"}
	expectedMap := map[string]armsubscriptions.Location{
		"East US": locations[1],
		"West US": locations[0],
	}

	names, locationMap := common.BuildAzureResourceGroupLocationListAndMap(locations)
	require.Equal(t, expectedNames, names)
	require.Equal(t, expectedMap, locationMap)
}
