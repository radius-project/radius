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
	"fmt"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/sdk"
	ucp_provider "github.com/radius-project/radius/pkg/ucp/secret/secretprovider"
)

//go:generate mockgen -typed -destination=./mock_provider.go -package=providers -self_package github.com/radius-project/radius/pkg/recipes/terraform/config/providers github.com/radius-project/radius/pkg/recipes/terraform/config/providers Provider

// Provider is an interface for generating Terraform provider configurations.
type Provider interface {
	// BuildConfig generates the Terraform provider configuration for the provider.
	// Returns a map of Terraform provider name to values representing the provider configuration.
	// Returns an error if the provider configuration cannot be generated.
	BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error)
}

// GetUCPConfiguredTerraformProviders returns a map of Terraform provider names to provider config builder.
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
// The function also extracts secrets from the secrets data input and updates the provider configurations with secrets as applicable.
func GetRecipeProviderConfigs(ctx context.Context, envConfig *recipes.Configuration, secrets map[string]recipes.SecretData) (map[string][]map[string]any, error) {
	providerConfigs := make(map[string][]map[string]any)

	// If the provider is not configured, or has empty configuration, skip this iteration
	if envConfig != nil && envConfig.RecipeConfig.Terraform.Providers != nil {
		for provider, config := range envConfig.RecipeConfig.Terraform.Providers {
			if len(config) > 0 {
				configList := make([]map[string]any, 0)

				for _, configDetails := range config {
					// Create map for current config for current provider.
					currentProviderConfig := make(map[string]any)

					// Retrieve configuration details from 'AdditionalProperties' property and add to currentConfig.
					if len(configDetails.AdditionalProperties) > 0 {
						currentProviderConfig = configDetails.AdditionalProperties
					}

					// Extract secrets from provider configuration if they are present.
					secretsConfig, err := extractSecretsFromRecipeConfig(configDetails.Secrets, secrets)
					if err != nil {
						return nil, err
					}

					// Merge secrets with current provider configuration.
					for key, value := range secretsConfig {
						// If the key already exists in the provider configuration,
						// config value in secrets for the key currently will override config in
						// additionalProperties.
						currentProviderConfig[key] = value
					}

					if len(currentProviderConfig) > 0 {
						configList = append(configList, currentProviderConfig)
					}
				}

				providerConfigs[provider] = configList
			}
		}
	}

	return providerConfigs, nil
}

// extractSecretsFromRecipeConfig extracts secrets for env recipe configuration from the secrets data input and updates the currentConfig map.
func extractSecretsFromRecipeConfig(recipeConfigSecrets map[string]datamodel.SecretReference, secrets map[string]recipes.SecretData) (map[string]any, error) {
	secretsConfig := make(map[string]any)

	// Extract secrets from configDetails if they are present
	for secretName, secretReference := range recipeConfigSecrets {
		// Extract secret value from the secrets data input
		if secretIDs, ok := secrets[secretReference.Source]; ok {
			if secretValue, ok := secretIDs.Data[secretReference.Key]; ok {
				secretsConfig[secretName] = secretValue
			} else {
				return nil, fmt.Errorf("missing secret key in secret store id: %s", secretReference.Source)
			}
		} else {
			return nil, fmt.Errorf("missing secret store id: %s", secretReference.Source)
		}
	}

	return secretsConfig, nil
}
