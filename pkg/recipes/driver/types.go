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
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

const (
	TerraformAzureProvider            = "registry.terraform.io/hashicorp/azurerm"
	TerraformAWSProvider              = "registry.terraform.io/hashicorp/aws"
	TerraformKubernetesProvider       = "registry.terraform.io/hashicorp/kubernetes"
	PrivateRegistrySecretKey_Pat      = "pat"
	PrivateRegistrySecretKey_Username = "username"
)

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

	// FindSecretIDs gets the secret store resource ID references associated with git private terraform repository source.
	// In the future it will be extended to get secret references for provider secrets.
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
	Secrets map[string]map[string]string
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

// GetSecretStoreID returns secretstore resource ID associated with git private terraform repository source.
func GetSecretStoreID(envConfig recipes.Configuration, templatePath string) (string, error) {
	if strings.HasPrefix(templatePath, "git::") {
		url, err := GetGitURL(templatePath)
		if err != nil {
			return "", err
		}

		// get the secret store id associated with the git domain of the template path.
		return envConfig.RecipeConfig.Terraform.Authentication.Git.PAT[strings.TrimPrefix(url.Hostname(), "www.")].Secret, nil
	}
	return "", nil
}
