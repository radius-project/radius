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
	"time"

	"github.com/project-radius/radius/pkg/metrics"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	"github.com/project-radius/radius/pkg/recipes/driver"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// # Function Explanation
//
// NewEngine creates a new Engine to deploy recipe.
func NewEngine(options Options) *engine {
	return &engine{options: options}
}

var _ Engine = (*engine)(nil)

// Options represents the configuration loader and type of driver used to deploy recipe.
type Options struct {
	ConfigurationLoader configloader.ConfigurationLoader
	Drivers             map[string]driver.Driver
}

type engine struct {
	options Options
}

// # Function Explanation
//
// Execute loads the recipe definition from the environment, finds the driver associated with the recipe, loads the
// configuration associated with the recipe, and then executes the recipe using the driver. It returns a RecipeOutput and
// an error if one occurs.
func (e *engine) Execute(ctx context.Context, recipe recipes.ResourceMetadata) (*recipes.RecipeOutput, error) {
	executionStart := time.Now()

	// Load Recipe Definition from the environment.
	definition, err := e.options.ConfigurationLoader.LoadRecipe(ctx, &recipe)
	if err != nil {
		recordRecipeExecutionMetrics(ctx, definition.Driver, executionStart, "LOAD_RECIPE_ERROR")
		return nil, err
	}

	driver, ok := e.options.Drivers[definition.Driver]
	if !ok {
		recordRecipeExecutionMetrics(ctx, definition.Driver, executionStart, "DRIVER_NOT_FOUND")
		return nil, fmt.Errorf("could not find driver %s", definition.Driver)
	}

	configuration, err := e.options.ConfigurationLoader.LoadConfiguration(ctx, recipe)
	if err != nil {
		recordRecipeExecutionMetrics(ctx, definition.Driver, executionStart, "LOAD_CONFIGURATION_ERROR")
		return nil, err
	}

	res, err := driver.Execute(ctx, *configuration, recipe, *definition)
	if err != nil {
		recordRecipeExecutionMetrics(ctx, definition.Driver, executionStart, "EXECUTE_ERROR")
		return nil, err
	}

	recordRecipeExecutionMetrics(ctx, definition.Driver, executionStart, "SUCCESS")

	return res, nil
}

// # Function Explanation
//
// Delete calls the Delete method of the driver specified in the recipe definition to delete the output resources.
func (e *engine) Delete(ctx context.Context, recipe recipes.ResourceMetadata, outputResources []rpv1.OutputResource) error {
	deletionStart := time.Now()

	// Load Recipe Definition from the environment.
	definition, err := e.options.ConfigurationLoader.LoadRecipe(ctx, &recipe)
	if err != nil {
		recordRecipeDeletionMetrics(ctx, definition.Driver, deletionStart, "LOAD_RECIPE_ERROR")
		return err
	}

	// Determine Recipe driver type
	driver, ok := e.options.Drivers[definition.Driver]
	if !ok {
		recordRecipeDeletionMetrics(ctx, definition.Driver, deletionStart, "DRIVER_NOT_FOUND")
		return fmt.Errorf("could not find driver %s", definition.Driver)
	}

	err = driver.Delete(ctx, outputResources)
	if err != nil {
		recordRecipeDeletionMetrics(ctx, definition.Driver, deletionStart, "DELETE_ERROR")
		return err
	}

	recordRecipeDeletionMetrics(ctx, definition.Driver, deletionStart, "SUCCESS")

	return nil
}

func recordRecipeExecutionMetrics(ctx context.Context, driver string, executionStart time.Time, result string) {
	// Record the execution duration.
	metrics.DefaultRecipeEngineMetrics.
		RecordRecipeExecutionDuration(ctx, executionStart, metrics.GenerateDriverAttribute(driver),
			metrics.GenerateRecipeExecutionResultAttribute(result))

	// Record the execution count.
	metrics.DefaultRecipeEngineMetrics.
		RecordRecipeExecution(ctx, metrics.GenerateDriverAttribute(driver),
			metrics.GenerateRecipeExecutionResultAttribute(result))
}

func recordRecipeDeletionMetrics(ctx context.Context, driver string, deletionStart time.Time, result string) {
	// Record the execution duration.
	metrics.DefaultRecipeEngineMetrics.
		RecordRecipeDeletionDuration(ctx, deletionStart, metrics.GenerateDriverAttribute(driver),
			metrics.GenerateRecipeExecutionResultAttribute(result))

	// Record the execution count.
	metrics.DefaultRecipeEngineMetrics.
		RecordRecipeDeletion(ctx, metrics.GenerateDriverAttribute(driver),
			metrics.GenerateRecipeExecutionResultAttribute(result))
}
