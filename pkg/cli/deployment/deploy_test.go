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

package deployment

import (
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/stretchr/testify/require"
)

func Test_GetProviderConfigs(t *testing.T) {
	resourceDeploymentClient := ResourceDeploymentClient{
		RadiusResourceGroup: "testrg",
	}
	options := clients.DeploymentOptions{
		Providers: &clients.Providers{},
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
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
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
		Providers: &clients.Providers{
			Azure: &clients.AzureProvider{
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
			Scope: "/planes/radius/local/resourceGroups/" + "testrg",
		},
	}

	providerConfig := resourceDeploymentClient.GetProviderConfigs(options)
	require.Equal(t, providerConfig, expectedConfig)
}
