// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"testing"

	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/stretchr/testify/require"
)

func Test_GetProviderConfigs(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
		// DeploymentsClient:          *armresources.DeploymentsClient,
		// DeploymentOperationsClient: *armresources.DeploymentOperationsClient,
		AzProvider: &workspaces.AzureProvider{},
	}

	var expectedConfig clientv2.ProviderConfig

	expectedConfig.Radius = &clientv2.Radius{
		Type: "Radius",
		Value: clientv2.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &clientv2.Deployments{
		Type: "Microsoft.Resources",
		Value: clientv2.Value{
			Scope: "/planes/deployments/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs()
	require.Equal(t, providerConfig, expectedConfig)
}

func Test_GetProviderConfigsWithAzProvider(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
		// DeploymentsClient:          *armresources.DeploymentsClient,
		// DeploymentOperationsClient: *armresources.DeploymentOperationsClient,
		AzProvider: &workspaces.AzureProvider{
			SubscriptionID: "dummy",
			ResourceGroup:  "azrg",
		},
	}

	var expectedConfig clientv2.ProviderConfig

	expectedConfig.Az = &clientv2.Az{
		Type: "AzureResourceManager",
		Value: clientv2.Value{
			Scope: "/subscriptions/dummy/resourceGroups/" + "azrg",
		},
	}

	expectedConfig.Radius = &clientv2.Radius{
		Type: "Radius",
		Value: clientv2.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &clientv2.Deployments{
		Type: "Microsoft.Resources",
		Value: clientv2.Value{
			Scope: "/planes/deployments/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs()
	require.Equal(t, providerConfig, expectedConfig)
}
