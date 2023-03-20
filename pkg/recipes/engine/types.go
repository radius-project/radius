// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package engine

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/recipes"
)

func NewEngine(options Options) *Engine {
	return &Engine{options: options}
}

var _ recipes.Engine = (*Engine)(nil)

type Options struct {
	ConfigurationLoader recipes.ConfigurationLoader
	Drivers             map[string]recipes.Driver
}

type Engine struct {
	options Options
}

// Execute implements recipes.Engine
func (e *Engine) Execute(ctx context.Context, recipe recipes.RecipeMetadata) (*recipes.RecipeResult, error) {
	// Resolve definition from repository
	definition, err := e.options.ConfigurationLoader.Lookup(ctx, recipe)
	if err != nil {
		return nil, err
	}

	driver, ok := e.options.Drivers[definition.Driver]
	if !ok {
		return nil, fmt.Errorf("could not find driver %s", definition.Driver)
	}

	configuration, err := e.options.ConfigurationLoader.Load(ctx, recipe)
	if err != nil {
		return nil, err
	}

	return driver.Execute(ctx, *configuration, recipe, *definition)
}
