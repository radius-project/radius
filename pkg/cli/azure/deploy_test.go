// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"testing"

	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/stretchr/testify/require"
)

func Test_GetProviderConfigs(t *testing.T) {

	resourceDeploymentClient := ResouceDeploymentClient{
		RadiusResourceGroup: "testrg",
		Client:              clients.ResourceDeploymentClient{},
		OperationsClient:    clients.ResourceDeploymentOperationsClient{},
		AzProvider:          &workspaces.AzureProvider{},
	}

	var expectedConfig providers.ProviderConfig

	expectedConfig.Radius = &providers.Radius{
		Type: "Radius",
		Value: providers.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &providers.Deployments{
		Type: "Microsoft.Resources",
		Value: providers.Value{
			Scope: "/planes/deployments/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs()
	require.Equal(t, providerConfig, expectedConfig)
}

func Test_GetProviderConfigsWithAzProvider(t *testing.T) {

	resourceDeploymentClient := ResouceDeploymentClient{
		RadiusResourceGroup: "testrg",
		Client:              clients.ResourceDeploymentClient{},
		OperationsClient:    clients.ResourceDeploymentOperationsClient{},
		AzProvider: &workspaces.AzureProvider{
			SubscriptionID: "dummy",
			ResourceGroup:  "azrg",
		},
	}

	var expectedConfig providers.ProviderConfig

	expectedConfig.Az = &providers.Az{
		Type: "AzureResourceManager",
		Value: providers.Value{
			Scope: "/subscriptions/dummy/resourceGroups/" + "azrg",
		},
	}

	expectedConfig.Radius = &providers.Radius{
		Type: "Radius",
		Value: providers.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &providers.Deployments{
		Type: "Microsoft.Resources",
		Value: providers.Value{
			Scope: "/planes/deployments/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs()
	require.Equal(t, providerConfig, expectedConfig)
}
