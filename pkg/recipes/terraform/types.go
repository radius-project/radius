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

package terraform

import (
	"context"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/terraform/config/providers"
)

//go:generate mockgen -destination=./mock_executor.go -package=terraform -self_package github.com/project-radius/radius/pkg/recipes/terraform github.com/project-radius/radius/pkg/recipes/terraform TerraformExecutor

type TerraformExecutor interface {
	// Deploy installs terraform and runs terraform init and apply on the terraform module referenced by the recipe using terraform-exec.
	Deploy(ctx context.Context, options Options) (*recipes.RecipeOutput, error)
}

// Options represents the options required to build inputs to interact with Terraform.
type Options struct {
	// RootDir is the root directory of where Terraform is installed and executed for a specific recipe deployment/deletion request.
	RootDir string

	// EnvConfig is the kubernetes runtime and cloud provider configuration for the Radius environment in which the application consuming the terraform recipe will be deployed.
	EnvConfig *recipes.Configuration

	// EnvRecipe is the recipe metadata associated with the Radius environment in which the application consuming the terraform recipe will be deployed.
	EnvRecipe *recipes.EnvironmentDefinition

	// ResourceRecipe is recipe metadata associated with the Radius resource deploying the Terraform recipe.
	ResourceRecipe *recipes.ResourceMetadata

	// Providers represent Terraform providers for which Radius generates custom provider configurations.
	// For example, the Azure subscription id is included in Azure provider config using Environment's scope.
	Providers map[string]providers.Provider
}
