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

package clients

import "fmt"

const (
	// ProviderTypeAzure is used to specify the provider configuration for Azure resources.
	ProviderTypeAzure = "AzureResourceManager"
	// ProviderTypeAWS is used to specify the provider configuration for AWS resources.
	ProviderTypeAWS = "AWS"
	// ProviderTypeDeployments is used to specify the provider configuration for Bicep modules.
	ProviderTypeDeployments = "Microsoft.Resources"
	// ProviderTypeRadius is used to specify the provider configuration for Radius resources.
	ProviderTypeRadius = "Radius"
)

// NewDefaultProviderConfig creates a ProviderConfig instance with two fields, Deployments and Radius, and sets their values
// based on the resourceGroup parameter. The default config will include configuration for Radius resources, Kubernetes resources,
// and Bicep modules. AWS and Azure resources must be added separately.
func NewDefaultProviderConfig(resourceGroup string) ProviderConfig {
	config := ProviderConfig{
		Deployments: &Deployments{
			Type: ProviderTypeDeployments,
			Value: Value{
				Scope: constructRadiusDeploymentScope(resourceGroup),
			},
		},
		Radius: &Radius{
			Type: ProviderTypeRadius,
			Value: Value{
				Scope: constructRadiusDeploymentScope(resourceGroup),
			},
		},
	}

	return config
}

// GenerateProviderConfig generates a ProviderConfig object based on the given scopes.
func GenerateProviderConfig(resourceGroup, awsScope, azureScope string) ProviderConfig {
	providerConfig := ProviderConfig{}
	if awsScope != "" {
		providerConfig.AWS = &AWS{
			Type: ProviderTypeAWS,
			Value: Value{
				Scope: awsScope,
			},
		}
	}
	if azureScope != "" {
		providerConfig.Az = &Az{
			Type: ProviderTypeAzure,
			Value: Value{
				Scope: azureScope,
			},
		}
	}
	if resourceGroup != "" {
		providerConfig.Radius = &Radius{
			Type: ProviderTypeRadius,
			Value: Value{
				Scope: constructRadiusDeploymentScope(resourceGroup),
			},
		}
		providerConfig.Deployments = &Deployments{
			Type: ProviderTypeDeployments,
			Value: Value{
				Scope: constructRadiusDeploymentScope(resourceGroup),
			},
		}
	}

	return providerConfig
}

// constructRadiusDeploymentScope constructs the scope for Radius deployments.
func constructRadiusDeploymentScope(group string) string {
	return fmt.Sprintf("/planes/radius/local/resourceGroups/%s", group)
}
