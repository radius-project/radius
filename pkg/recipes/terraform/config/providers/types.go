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

package providers

import (
	"context"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/sdk"
	ucp_provider "github.com/radius-project/radius/pkg/ucp/secret/provider"
)

//go:generate mockgen -destination=./mock_provider.go -package=providers -self_package github.com/radius-project/radius/pkg/recipes/terraform/config/providers github.com/radius-project/radius/pkg/recipes/terraform/config/providers Provider

// Provider is an interface for generating Terraform provider configurations.
type Provider interface {
	// BuildConfig generates the Terraform provider configuration for the provider.
	// Returns a map of Terraform provider name to values representing the provider configuration.
	// Returns an error if the provider configuration cannot be generated.
	BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error)
}

// GetUCPConfiguredTerraformProviders returns a map of Terraform provider names with configuration details stored in UCP, to provider config builder.
// These providers represent Terraform providers for which Radius generates custom provider configurations based on credentials stored with UCP
// and providers configured on the Radius environment. For example, the Azure subscription id is added to Azure provider config using Radius Environment's Azure provider scope.
func GetUCPConfiguredTerraformProviders(ucpConn sdk.Connection, secretProvider *ucp_provider.SecretProvider) map[string]Provider {
	return map[string]Provider{
		AWSProviderName:        NewAWSProvider(ucpConn, secretProvider),
		AzureProviderName:      NewAzureProvider(ucpConn, secretProvider),
		KubernetesProviderName: &kubernetesProvider{},
	}
}

// GetRecipeProviderConfigs returns the Terraform provider configurations for Terraform providers
// specified under the RecipeConfig/Terraform/Providers section under environment configuration.
func GetRecipeProviderConfigs(ctx context.Context, envConfig *recipes.Configuration) map[string]any {
	providerConfigs := make(map[string]any)

	// If the provider is not configured, or has empty configuration, skip this iteration
	if envConfig != nil && envConfig.RecipeConfig.Terraform.Providers != nil {
		for provider, config := range envConfig.RecipeConfig.Terraform.Providers {
			if len(config) > 0 {
				configList := make([]any, 0)

				// Retrieve configuration details from 'AdditionalProperties' property and add to the list.
				for _, configDetails := range config {
					configList = append(configList, configDetails.AdditionalProperties)
				}

				providerConfigs[provider] = configList
			}
		}
	}

	return providerConfigs
}
