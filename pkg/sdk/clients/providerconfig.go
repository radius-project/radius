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

// NewDefaultProviderConfig creates a new ProviderConfig for use with ResourceDeploymentsClient.
//
// The default config will include configuration for Radius resources, Kuberenetes resources, and Bicep modules.
// AWS and Azure resources must be added separately.
func NewDefaultProviderConfig(resourceGroup string) ProviderConfig {
	config := ProviderConfig{
		Deployments: &Deployments{
			Type: "Microsoft.Resources",
			Value: Value{
				Scope: "/planes/deployments/local/resourceGroups/" + resourceGroup,
			},
		},
		Radius: &Radius{
			Type: "Radius",
			Value: Value{
				Scope: "/planes/radius/local/resourceGroups/" + resourceGroup,
			},
		},
	}

	return config
}
