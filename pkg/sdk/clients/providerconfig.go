// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
