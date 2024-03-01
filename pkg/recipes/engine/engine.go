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

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/metrics"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	recipedriver "github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/util"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// NewEngine creates a new Engine to deploy recipe.
func NewEngine(options Options) *engine {
	return &engine{options: options}
}

var _ Engine = (*engine)(nil)

// Options represents the configuration loader and type of driver used to deploy recipe.
type Options struct {
	ConfigurationLoader configloader.ConfigurationLoader
	SecretsLoader       configloader.SecretsLoader
	Drivers             map[string]recipedriver.Driver
}

type engine struct {
	options Options
}

// Execute loads the recipe definition from the environment, finds the driver associated with the recipe, loads the
// configuration associated with the recipe, and then executes the recipe using the driver. It returns a RecipeOutput and
// an error if one occurs.
func (e *engine) Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error) {
	executionStart := time.Now()
	result := metrics.SuccessfulOperationState

	recipeOutput, definition, err := e.executeCore(ctx, opts.Recipe, opts.PreviousState)
	if err != nil {
		result = metrics.FailedOperationState
		if recipes.GetErrorDetails(err) != nil {
			result = recipes.GetErrorDetails(err).Code
		}
	}

	metrics.DefaultRecipeEngineMetrics.RecordRecipeOperationDuration(ctx, executionStart,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationExecute, opts.Recipe.Name,
			definition, result))

	return recipeOutput, err
}

// executeCore function is the core logic of the Execute function.
// Any changes to the core logic of the Execute function should be made here.
func (e *engine) executeCore(ctx context.Context, recipe recipes.ResourceMetadata, prevState []string) (*recipes.RecipeOutput, *recipes.EnvironmentDefinition, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	configuration, err := e.options.ConfigurationLoader.LoadConfiguration(ctx, recipe)
	if err != nil {
		return nil, nil, recipes.NewRecipeError(recipes.RecipeConfigurationFailure, err.Error(), util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	// No need to try executing the recipe if it's a simulated environment.
	if configuration.Simulated {
		logger.Info("simulated environment enabled, skipping deployment")
		return nil, nil, nil
	}

	definition, driver, err := e.getDriver(ctx, recipe)
	if err != nil {
		return nil, nil, err
	}

	// Retrieves the secret store id from the recipes configuration for the terraform module source of type git.
	// secretStoreID returned will be an empty string for other types.
	secretStore, err := recipes.GetSecretStoreID(*configuration, definition.TemplatePath)
	if err != nil {
		return nil, nil, err
	}

	// Retrieves the secret values from the secret store ID provided.
	secrets := v20231001preview.SecretStoresClientListSecretsResponse{}
	if secretStore != "" {
		secrets, err = e.options.SecretsLoader.LoadSecrets(ctx, secretStore)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch secrets from the secret store resource id %s for Terraform recipe %s deployment: %w", secretStore, definition.TemplatePath, err)
		}
	}

	res, err := driver.Execute(ctx, recipedriver.ExecuteOptions{
		BaseOptions: recipedriver.BaseOptions{
			Configuration: *configuration,
			Recipe:        recipe,
			Definition:    *definition,
			Secrets:       secrets,
		},
		PrevState: prevState,
	})
	if err != nil {
		return nil, definition, err
	}

	return res, definition, nil
}

// Delete calls the Delete method of the driver specified in the recipe definition to delete the output resources.
func (e *engine) Delete(ctx context.Context, opts DeleteOptions) error {
	deletionStart := time.Now()
	result := metrics.SuccessfulOperationState

	definition, err := e.deleteCore(ctx, opts.Recipe, opts.OutputResources)
	if err != nil {
		result = metrics.FailedOperationState
		if recipes.GetErrorDetails(err) != nil {
			result = recipes.GetErrorDetails(err).Code
		}
	}

	metrics.DefaultRecipeEngineMetrics.RecordRecipeOperationDuration(ctx, deletionStart,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDelete, opts.Recipe.Name,
			definition, result))

	return err
}

// deleteCore function is the core logic of the Delete function.
// Any changes to the core logic of the Delete function should be made here.
func (e *engine) deleteCore(ctx context.Context, recipe recipes.ResourceMetadata, outputResources []rpv1.OutputResource) (*recipes.EnvironmentDefinition, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	configuration, err := e.options.ConfigurationLoader.LoadConfiguration(ctx, recipe)
	if err != nil {
		return nil, err
	}

	if configuration.Simulated {
		logger.Info("simulated environment enabled, skipping deleting resources")
		return nil, nil
	}

	definition, driver, err := e.getDriver(ctx, recipe)
	if err != nil {
		return nil, err
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

// Gets the Recipe metadata and parameters from Recipe's template path.
func (e *engine) GetRecipeMetadata(ctx context.Context, recipeDefinition recipes.EnvironmentDefinition) (map[string]any, error) {
	recipeData, err := e.getRecipeMetadataCore(ctx, recipeDefinition)
	if err != nil {
		return nil, err
	}

	return recipeData, nil
}

// getRecipeMetadataCore function is the core logic of the GetRecipeMetadata function.
// Any changes to the core logic of the GetRecipeMetadata function should be made here.
func (e *engine) getRecipeMetadataCore(ctx context.Context, recipeDefinition recipes.EnvironmentDefinition) (map[string]any, error) {
	// Determine Recipe driver type
	driver, ok := e.options.Drivers[recipeDefinition.Driver]
	if !ok {
		return nil, fmt.Errorf("could not find driver %s", recipeDefinition.Driver)
	}

	return driver.GetRecipeMetadata(ctx, recipedriver.BaseOptions{
		Recipe:     recipes.ResourceMetadata{},
		Definition: recipeDefinition,
	})
}

func (e *engine) getDriver(ctx context.Context, recipeMetadata recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, recipedriver.Driver, error) {
	// Load Recipe Definition from the environment.
	definition, err := e.options.ConfigurationLoader.LoadRecipe(ctx, &recipeMetadata)
	if err != nil {
		return nil, nil, err
	}

	// Determine Recipe driver type
	driver, ok := e.options.Drivers[definition.Driver]
	if !ok {
		err := fmt.Errorf("could not find driver `%s`", definition.Driver)
		return nil, nil, recipes.NewRecipeError(recipes.RecipeDriverNotFoundFailure, err.Error(), util.RecipeSetupError, recipes.GetErrorDetails(err))
	}
	return definition, driver, nil
}
