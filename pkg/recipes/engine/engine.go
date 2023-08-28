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
	"fmt"
	reflect "reflect"
	"time"

	"github.com/project-radius/radius/pkg/metrics"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	recipedriver "github.com/project-radius/radius/pkg/recipes/driver"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// NewEngine creates a new Engine to deploy recipe.
func NewEngine(options Options) *engine {
	return &engine{options: options}
}

var _ Engine = (*engine)(nil)

// Options represents the configuration loader and type of driver used to deploy recipe.
type Options struct {
	ConfigurationLoader configloader.ConfigurationLoader
	Drivers             map[string]recipedriver.Driver
}

type engine struct {
	options Options
}

// Execute loads the recipe definition from the environment, finds the driver associated with the recipe, loads the
// configuration associated with the recipe, and then executes the recipe using the driver. It returns a RecipeOutput and
// an error if one occurs.
func (e *engine) Execute(ctx context.Context, recipe recipes.ResourceMetadata) (*recipes.RecipeOutput, error) {
	executionStart := time.Now()
	result := metrics.SuccessfulOperationState

	recipeOutput, definition, err := e.executeCore(ctx, recipe)
	if err != nil {
		result = metrics.FailedOperationState
	}

	metrics.DefaultRecipeEngineMetrics.RecordRecipeOperationDuration(ctx, executionStart,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationExecute, recipe.Name,
			definition, result))

	return recipeOutput, err
}

// executeCore function is the core logic of the Execute function.
// Any changes to the core logic of the Execute function should be made here.
func (e *engine) executeCore(ctx context.Context, recipe recipes.ResourceMetadata) (*recipes.RecipeOutput, *recipes.EnvironmentDefinition, error) {
	definition, driver, err := e.getRecipeDefinitionAndDriver(ctx, recipe)
	if err != nil {
		return nil, nil, err
	}

	configuration, err := e.options.ConfigurationLoader.LoadConfiguration(ctx, recipe)
	if err != nil {
		return nil, definition, err
	}

	res, err := driver.Execute(ctx, recipedriver.ExecuteOptions{
		BaseOptions: recipedriver.BaseOptions{
			Configuration: *configuration,
			Recipe:        recipe,
			Definition:    *definition,
		},
	})
	if err != nil {
		return nil, definition, err
	}

	return res, definition, nil
}

// Delete calls the Delete method of the driver specified in the recipe definition to delete the output resources.
func (e *engine) Delete(ctx context.Context, recipe recipes.ResourceMetadata, outputResources []rpv1.OutputResource) error {
	deletionStart := time.Now()
	result := metrics.SuccessfulOperationState

	definition, err := e.deleteCore(ctx, recipe, outputResources)
	if err != nil {
		result = metrics.FailedOperationState
	}

	metrics.DefaultRecipeEngineMetrics.RecordRecipeOperationDuration(ctx, deletionStart,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDelete, recipe.Name,
			definition, result))

	return err
}

// deleteCore function is the core logic of the Delete function.
// Any changes to the core logic of the Delete function should be made here.
func (e *engine) deleteCore(ctx context.Context, recipe recipes.ResourceMetadata, outputResources []rpv1.OutputResource) (*recipes.EnvironmentDefinition, error) {
	definition, driver, err := e.getRecipeDefinitionAndDriver(ctx, recipe)
	if err != nil {
		return nil, err
	}

	configuration, err := e.options.ConfigurationLoader.LoadConfiguration(ctx, recipe)
	if err != nil {
		return definition, err
	}

	err = driver.Delete(ctx, recipedriver.DeleteOptions{
		BaseOptions: recipedriver.BaseOptions{
			Configuration: *configuration,
			Recipe:        recipe,
			Definition:    *definition,
		},
		OutputResources: outputResources,
	})
	if err != nil {
		return definition, err
	}

	return definition, nil
}

func (e *engine) GarbageCollectResources(ctx context.Context, recipe recipes.ResourceMetadata, diff []rpv1.OutputResource) error {
	if reflect.DeepEqual(recipes.ResourceMetadata{}, recipe) {
		return nil
	}
	_, driver, err := e.getRecipeDefinitionAndDriver(ctx, recipe)
	if err != nil {
		return err
	}
	return driver.GarbageCollectResources(ctx, diff)
}

func (e *engine) getRecipeDefinitionAndDriver(ctx context.Context, recipe recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, recipedriver.Driver, error) {
	// Load Recipe Definition from the environment.
	definition, err := e.options.ConfigurationLoader.LoadRecipe(ctx, &recipe)
	if err != nil {
		return nil, nil, err
	}

	// Determine Recipe driver type
	driver, ok := e.options.Drivers[definition.Driver]
	if !ok {
		return nil, nil, fmt.Errorf("could not find driver %s", definition.Driver)
	}
	return definition, driver, nil
}
