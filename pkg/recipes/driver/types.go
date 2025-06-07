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

package driver

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

const (
	DefaultTerraformRegistry           = "registry.terraform.io"
	DefaultTerraformAzureProvider      = "hashicorp/azurerm"
	DefaultTerraformAWSProvider        = "hashicorp/aws"
	DefaultTerraformKubernetesProvider = "hashicorp/kubernetes"

	PrivateRegistrySecretKey_Pat      = "pat"
	PrivateRegistrySecretKey_Username = "username"
)

// GetTerraformProviderFullName returns the full provider name including registry
func GetTerraformProviderFullName(registry, provider string) string {
	return fmt.Sprintf("%s/%s", registry, provider)
}

// GetTerraformRegistry returns the registry to use based on configuration
func GetTerraformRegistry(config recipes.Configuration) string {
	if config.RecipeConfig.Terraform.Registry != nil && config.RecipeConfig.Terraform.Registry.Mirror != "" {
		return config.RecipeConfig.Terraform.Registry.Mirror
	}
	return DefaultTerraformRegistry
}

// GetTerraformProviderName returns the provider name to use based on configuration
func GetTerraformProviderName(config recipes.Configuration, defaultProvider, providerName string) string {
	if config.RecipeConfig.Terraform.Registry != nil && config.RecipeConfig.Terraform.Registry.ProviderMappings != nil {
		if mapping, exists := config.RecipeConfig.Terraform.Registry.ProviderMappings[defaultProvider]; exists {
			return mapping
		}
	}
	return providerName
}

// Driver is an interface to implement recipe deployment and recipe resources deletion.
//
//go:generate mockgen -typed -destination=./mock_driver.go -package=driver -self_package github.com/radius-project/radius/pkg/recipes/driver github.com/radius-project/radius/pkg/recipes/driver Driver
type Driver interface {
	// Execute fetches the recipe contents and deploys the recipe and returns deployed resources, secrets and values.
	Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error)

	// Delete handles deletion of output resources for the recipe deployment.
	Delete(ctx context.Context, opts DeleteOptions) error

	// Gets the Recipe metadata and parameters from Recipe's template path
	GetRecipeMetadata(ctx context.Context, opts BaseOptions) (map[string]any, error)
}

// DriverWithSecrets is an optional interface and used when the driver needs to load secrets for recipe deployment.
//
//go:generate mockgen -typed -destination=./mock_driver_with_secrets.go -package=driver -self_package github.com/radius-project/radius/pkg/recipes/driver github.com/radius-project/radius/pkg/recipes/driver DriverWithSecrets
type DriverWithSecrets interface {
	// Driver is an interface to implement recipe deployment and recipe resources deletion.
	Driver

	// FindSecretIDs retrieves a map of secret store resource IDs and their corresponding secret keys for secrets required for recipe deployment.
	FindSecretIDs(ctx context.Context, config recipes.Configuration, definition recipes.EnvironmentDefinition) (secretIDs map[string][]string, err error)
}

// BaseOptions is the base options for the driver operations.
type BaseOptions struct {
	// Configuration is the configuration for the recipe.
	Configuration recipes.Configuration

	// Recipe is the recipe metadata.
	Recipe recipes.ResourceMetadata

	// Definition is the environment definition for the recipe.
	Definition recipes.EnvironmentDefinition

	// Secrets represents a map of secrets required for recipe execution.
	// The outer map's key represents the secretStoreIDs while
	// while the inner map's key-value pairs represent the [secretKey]secretValue.
	// Example:
	// Secrets{
	//     "secretStoreID1": {
	//         "apiKey": "value1",
	//         "apiSecret": "value2",
	//     },
	//     "secretStoreID2": {
	//         "accessKey": "accessKey123",
	//         "secretKey": "secretKeyXYZ",
	//     },
	// }
	Secrets map[string]recipes.SecretData
}

// ExecuteOptions is the options for the Execute method.
type ExecuteOptions struct {
	BaseOptions
	// Previously deployed state of output resource IDs.
	PrevState []string
}

// DeleteOptions is the options for the Delete method.
type DeleteOptions struct {
	BaseOptions

	// OutputResources is the list of output resources for the recipe.
	OutputResources []rpv1.OutputResource
}
