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
	"github.com/golang/mock/gomock"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_enterAzureCloudProvider(t *testing.T) {
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
		Name: to.Ptr("test-resource-group"),
	}

	// selectAzureSubscription
	setAzureSubscriptions(client, &azure.SubscriptionResult{Default: &subscription, Subscriptions: []azure.Subscription{subscription}})
	setAzureSubscriptionConfirmPrompt(prompter, subscription.Name, prompt.ConfirmYes)

	// selectAzureResourceGroup
	setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmNo)
	setAzureResourceGroups(client, subscription.ID, []armresources.ResourceGroup{resourceGroup})
	setAzureResourceGroupPrompt(prompter, []string{*resourceGroup.Name}, *resourceGroup.Name)

	// service principal
	setAzureServicePrincipalAppIDPrompt(prompter, "service-principal-app-id")
	setAzureServicePrincipalPasswordPrompt(prompter, "service-principal-password")
	setAzureServicePrincipalTenantIDPrompt(prompter, "service-principal-tenant-id")

	options := initOptions{}
	provider, err := runner.enterAzureCloudProvider(context.Background(), &options)
	require.NoError(t, err)

	expected := &azure.Provider{
		SubscriptionID: subscription.ID,
		ResourceGroup:  *resourceGroup.Name,
		ServicePrincipal: &azure.ServicePrincipal{
			ClientID:     "service-principal-app-id",
			ClientSecret: "service-principal-password",
			TenantID:     "service-principal-tenant-id",
		},
	}
	require.Equal(t, expected, provider)

	expectedOutput := []any{output.LogOutput{
		Format: azureServicePrincipalCreateInstructionsFmt,
		Params: []any{subscription.ID, *resourceGroup.Name},
	}}
	require.Equal(t, expectedOutput, outputSink.Writes)
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
		runner := Runner{Prompter: prompter, azureClient: client}

		subscriptions := subscriptions
		subscriptions.Default = &subscriptions.Subscriptions[1]

		setAzureSubscriptions(client, &subscriptions)
		setAzureSubscriptionConfirmPrompt(prompter, subscriptions.Default.Name, prompt.ConfirmYes)

		selected, err := runner.selectAzureSubscription(context.Background())
		require.NoError(t, err)
		require.NotNil(t, selected)

		require.Equal(t, *subscriptions.Default, *selected)
	})

	t.Run("choose non-default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		client := azure.NewMockClient(ctrl)
		runner := Runner{Prompter: prompter, azureClient: client}

		subscriptions := subscriptions
		subscriptions.Default = &subscriptions.Subscriptions[1]

		setAzureSubscriptions(client, &subscriptions)
		setAzureSubscriptionConfirmPrompt(prompter, subscriptions.Default.Name, prompt.ConfirmNo)
		setAzureSubsubscriptionPrompt(prompter, subscriptionNames, subscriptions.Subscriptions[2].Name)

		selected, err := runner.selectAzureSubscription(context.Background())
		require.NoError(t, err)
		require.NotNil(t, selected)

		require.Equal(t, subscriptions.Subscriptions[2], *selected)
	})

	t.Run("no-default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		client := azure.NewMockClient(ctrl)
		runner := Runner{Prompter: prompter, azureClient: client}

		subscriptions := subscriptions
		subscriptions.Default = nil

		setAzureSubscriptions(client, &subscriptions)
		setAzureSubsubscriptionPrompt(prompter, subscriptionNames, subscriptions.Subscriptions[2].Name)

		selected, err := runner.selectAzureSubscription(context.Background())
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

	runner := Runner{}
	names, subscriptionMap := runner.buildAzureSubscriptionListAndMap(&subscriptions)
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
			Name: to.Ptr("b-test-resource-group1"),
		},
		{
			Name: to.Ptr("a-test-resource-group2"),
		},
		{
			Name: to.Ptr("c-test-resource-group3"),
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
			Name:        to.Ptr("westus"),
			DisplayName: to.Ptr("West US"),
		},
		{
			Name:        to.Ptr("eastus"),
			DisplayName: to.Ptr("East US"),
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
		runner := Runner{Prompter: prompter, azureClient: client, Output: &outputSink}

		setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmNo)
		setAzureResourceGroups(client, subscription.ID, resourceGroups)
		setAzureResourceGroupPrompt(prompter, resourceGroupNames, *resourceGroups[1].Name)

		name, err := runner.selectAzureResourceGroup(context.Background(), subscription)
		require.NoError(t, err)

		require.Equal(t, *resourceGroups[1].Name, name)
		require.Empty(t, outputSink.Writes)
	})

	t.Run("create new", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		prompter := prompt.NewMockInterface(ctrl)
		client := azure.NewMockClient(ctrl)
		outputSink := output.MockOutput{}
		runner := Runner{Prompter: prompter, azureClient: client, Output: &outputSink}

		setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmYes)
		setAzureResourceGroupNamePrompt(prompter, "test-resource-group")
		setAzureCheckResourceGroupExistence(client, subscription.ID, "test-resource-group", false)
		setAzureLocations(client, subscription.ID, locations)
		setSelectAzureResourceGroupLocationPrompt(prompter, locationDisplayNames, *locations[1].DisplayName)
		setAzureCreateOrUpdateResourceGroup(client, subscription.ID, "test-resource-group", *locations[1].Name)

		name, err := runner.selectAzureResourceGroup(context.Background(), subscription)
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
		runner := Runner{Prompter: prompter, azureClient: client, Output: &outputSink}

		setAzureResourceGroupCreatePrompt(prompter, prompt.ConfirmYes)
		setAzureResourceGroupNamePrompt(prompter, "test-resource-group")
		setAzureCheckResourceGroupExistence(client, subscription.ID, "test-resource-group", true)

		name, err := runner.selectAzureResourceGroup(context.Background(), subscription)
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
			Name: to.Ptr("b-test-resource-group1"),
		},
		{
			Name: to.Ptr("a-test-resource-group2"),
		},
		{
			Name: to.Ptr("c-test-resource-group3"),
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
	runner := Runner{Prompter: prompter, azureClient: client}

	setAzureResourceGroups(client, subscription.ID, resourceGroups)
	setAzureResourceGroupPrompt(prompter, resourceGroupNames, *resourceGroups[1].Name)

	name, err := runner.selectExistingAzureResourceGroup(context.Background(), subscription)
	require.NoError(t, err)

	require.Equal(t, *resourceGroups[1].Name, name)
}

func Test_buildAzureResourceGroupList(t *testing.T) {
	// Intentionally not in alphabetical order
	resourceGroups := []armresources.ResourceGroup{
		{
			Name: to.Ptr("b-test-resource-group1"),
		},
		{
			Name: to.Ptr("a-test-resource-group2"),
		},
		{
			Name: to.Ptr("c-test-resource-group3"),
		},
	}

	expectedNames := []string{"a-test-resource-group2", "b-test-resource-group1", "c-test-resource-group3"}

	runner := Runner{}
	names := runner.buildAzureResourceGroupList(resourceGroups)
	require.Equal(t, expectedNames, names)
}

func Test_enterAzureResourceGroupName(t *testing.T) {
	ctrl := gomock.NewController(t)
	prompter := prompt.NewMockInterface(ctrl)
	client := azure.NewMockClient(ctrl)
	runner := Runner{Prompter: prompter, azureClient: client}

	setAzureResourceGroupNamePrompt(prompter, "test-resource-group")

	name, err := runner.enterAzureResourceGroupName(context.Background())
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
			Name: to.Ptr("b-test-resource-group1"),
		},
		{
			Name: to.Ptr("a-test-resource-group2"),
		},
		{
			Name: to.Ptr("c-test-resource-group3"),
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
			Name:        to.Ptr("westus"),
			DisplayName: to.Ptr("West US"),
		},
		{
			Name:        to.Ptr("eastus"),
			DisplayName: to.Ptr("East US"),
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
	runner := Runner{Prompter: prompter, azureClient: client}

	setAzureLocations(client, subscription.ID, locations)
	setSelectAzureResourceGroupLocationPrompt(prompter, locationDisplayNames, *locations[1].DisplayName)

	location, err := runner.selectAzureResourceGroupLocation(context.Background(), subscription)
	require.NoError(t, err)
	require.Equal(t, locations[1], *location)
}

func Test_buildAzureResourceGroupLocationListAndMap(t *testing.T) {
	// Intentionally not in alphabetical order
	locations := []armsubscriptions.Location{
		{
			Name:        to.Ptr("westus"),
			DisplayName: to.Ptr("West US"),
		},
		{
			Name:        to.Ptr("eastus"),
			DisplayName: to.Ptr("East US"),
		},
	}

	expectedNames := []string{"East US", "West US"}
	expectedMap := map[string]armsubscriptions.Location{
		"East US": locations[1],
		"West US": locations[0],
	}

	runner := Runner{}
	names, locationMap := runner.buildAzureResourceGroupLocationListAndMap(locations)
	require.Equal(t, expectedNames, names)
	require.Equal(t, expectedMap, locationMap)
}
