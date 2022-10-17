// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"testing"

	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/stretchr/testify/require"
)

func Test_GetProviderConfigs(t *testing.T) {

	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
		Client:              clients.ResourceDeploymentClient{},
		OperationsClient:    clients.ResourceDeploymentOperationsClient{},
		AzProvider:          &workspaces.AzureProvider{},
	}

	var expectedConfig clients.ProviderConfig

	expectedConfig.Radius = &clients.Radius{
		Type: "Radius",
		Value: clients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &clients.Deployments{
		Type: "Microsoft.Resources",
		Value: clients.Value{
			Scope: "/planes/deployments/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs()
	require.Equal(t, providerConfig, expectedConfig)
}

func Test_GetProviderConfigsWithAzProvider(t *testing.T) {

	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
		Client:              clients.ResourceDeploymentClient{},
		OperationsClient:    clients.ResourceDeploymentOperationsClient{},
		AzProvider: &workspaces.AzureProvider{
			SubscriptionID: "dummy",
			ResourceGroup:  "azrg",
		},
	}

	var expectedConfig clients.ProviderConfig

	expectedConfig.Az = &clients.Az{
		Type: "AzureResourceManager",
		Value: clients.Value{
			Scope: "/subscriptions/dummy/resourceGroups/" + "azrg",
		},
	}

	expectedConfig.Radius = &clients.Radius{
		Type: "Radius",
		Value: clients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &clients.Deployments{
		Type: "Microsoft.Resources",
		Value: clients.Value{
			Scope: "/planes/deployments/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs()
	require.Equal(t, providerConfig, expectedConfig)
}
