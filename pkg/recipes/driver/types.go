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

	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

// Driver is an interface to implement recipe deployment and recipe resources deletion.
type Driver interface {
	// Execute fetches the recipe contents and deploys the recipe and returns deployed resources, secrets and values.
	Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutputResponse, error)

	// Delete handles deletion of output resources for the recipe deployment.
	Delete(ctx context.Context, opts DeleteOptions) error

	// Gets the Recipe metadata and parameters from Recipe's template path
	GetRecipeMetadata(ctx context.Context, opts BaseOptions) (map[string]any, error)
}

// BaseOptions is the base options for the driver operations.
type BaseOptions struct {
	// Configuration is the configuration for the recipe.
	Configuration recipes.Configuration

	// Recipe is the recipe metadata.
	Recipe recipes.ResourceMetadata

	// Definition is the environment definition for the recipe.
	Definition recipes.EnvironmentDefinition
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
