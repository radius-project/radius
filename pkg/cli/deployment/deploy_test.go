// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"testing"

	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	sdkclients "github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
)

func Test_GetProviderConfigs(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
	}
	options := clients.DeploymentOptions{
		Providers: &datamodel.Providers{},
	}

	var expectedConfig sdkclients.ProviderConfig

	expectedConfig.Radius = &sdkclients.Radius{
		Type: "Radius",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &sdkclients.Deployments{
		Type: "Microsoft.Resources",
		Value: sdkclients.Value{
			Scope: "/planes/deployments/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs(options)
	require.Equal(t, providerConfig, expectedConfig)
}

func Test_GetProviderConfigsWithAzProvider(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
		Client:              &sdkclients.ResourceDeploymentsClient{},
		OperationsClient:    &sdkclients.ResourceDeploymentOperationsClient{},
	}

	options := clients.DeploymentOptions{
		Providers: &datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "/subscriptions/dummy/resourceGroups/azrg",
			},
		},
	}

	var expectedConfig sdkclients.ProviderConfig

	expectedConfig.Az = &sdkclients.Az{
		Type: "AzureResourceManager",
		Value: sdkclients.Value{
			Scope: "/subscriptions/dummy/resourceGroups/" + "azrg",
		},
	}

	expectedConfig.Radius = &sdkclients.Radius{
		Type: "Radius",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}
	expectedConfig.Deployments = &sdkclients.Deployments{
		Type: "Microsoft.Resources",
		Value: sdkclients.Value{
			Scope: "/planes/deployments/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs(options)
	require.Equal(t, providerConfig, expectedConfig)
}
