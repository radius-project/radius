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

	"github.com/project-radius/radius/pkg/recipes"
)

//go:generate mockgen -destination=./mock_provider.go -package=providers -self_package github.com/project-radius/radius/pkg/recipes/terraform/config/providers github.com/project-radius/radius/pkg/recipes/terraform/config/providers Provider

// Provider is an interface for generating Terraform provider configurations.
type Provider interface {
	// BuildConfig generates the Terraform provider configuration for the provider.
	// Returns a map of Terraform provider name to values representing the provider configuration.
	// Returns an error if the provider configuration cannot be generated.
	BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error)
	BuildRequiredProvider() ProviderDefinition
}

// GetSupportedTerraformProviders returns a map of Terraform provider names to provider config builder.
// Providers represent Terraform providers for which Radius generates custom provider configurations.
// For example, the Azure subscription id is added to Azure provider config using Radius Environment's Azure provider scope.
func GetSupportedTerraformProviders() map[string]Provider {
	return map[string]Provider{
		AWSProviderName:        NewAWSProvider(),
		AzureProviderName:      NewAzureProvider(),
		KubernetesProviderName: NewKubernetesProvider(),
	}
}

type ProviderDefinition struct {
	Source  string `json:"source"`
	Version string `json:"version"`
}
