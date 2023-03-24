// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package engine

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	"github.com/project-radius/radius/pkg/recipes/driver"
)

func NewEngine(options Options) *engine {
	return &engine{options: options}
}

var _ Engine = (*engine)(nil)

type Options struct {
	ConfigurationLoader configloader.ConfigurationLoader
	Drivers             map[string]driver.Driver
}

type engine struct {
	options Options
}

// Execute gathers environment configuration and recipe definition and calls the driver to deploy the recipe.
func (e *engine) Execute(ctx context.Context, recipe recipes.RecipeMetadata) (*recipes.RecipeResult, error) {
	// Resolve definition from repository
	definition, err := e.options.ConfigurationLoader.LoadRecipe(ctx, recipe)
	if err != nil {
		return nil, err
	}

	driver, ok := e.options.Drivers[definition.Driver]
	if !ok {
		return nil, fmt.Errorf("could not find driver %s", definition.Driver)
	}

	configuration, err := e.options.ConfigurationLoader.LoadConfiguration(ctx, recipe)
	if err != nil {
		return nil, err
	}

	return driver.Execute(ctx, *configuration, recipe, *definition)
}
