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

package engine

import (
	"context"

	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

//go:generate mockgen -destination=./mock_engine.go -package=engine -self_package github.com/radius-project/radius/pkg/recipes/engine github.com/radius-project/radius/pkg/recipes/engine Engine

type Engine interface {
	// Execute gathers environment configuration, recipe definition and calls the driver to deploy the recipe.
	// prevState is added to the driver execute options, which is used to get the obsolete resources for cleanup. It consists list of recipe output resource IDs that were created in the previous deployment.
	Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error)

	// Delete handles deletion of output resources for the recipe deployment.
	Delete(ctx context.Context, opts DeleteOptions) error

	// Gets the Recipe metadata and parameters from Recipe's template path
	GetRecipeMetadata(ctx context.Context, recipeDefinition recipes.EnvironmentDefinition) (map[string]any, error)
}

// BaseOptions is the base options for the engine operations.
type BaseOptions struct {
	// Recipe is the recipe metadata.
	Recipe recipes.ResourceMetadata
}

// ExecuteOptions is the options for the Execute method.
type ExecuteOptions struct {
	BaseOptions
	// PreviousState represents previously deployed state of output resource IDs.
	PreviousState []string
	// Simulated is the flag to indicate if the execution is a simulation.
	Simulated bool
}

// DeleteOptions is the options for the Delete method.
type DeleteOptions struct {
	BaseOptions

	// OutputResources is the list of output resources for the recipe.
	OutputResources []rpv1.OutputResource
}
