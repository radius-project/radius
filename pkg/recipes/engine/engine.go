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

	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	"github.com/project-radius/radius/pkg/recipes/driver"
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
	Drivers             map[string]driver.Driver
}

type engine struct {
	options Options
}

// Execute gathers environment configuration and recipe definition and calls the driver to deploy the recipe.
func (e *engine) Execute(ctx context.Context, recipe recipes.ResourceMetadata) (*recipes.RecipeOutput, error) {
	// Load Recipe Definition from the environment.
	definition, err := e.options.ConfigurationLoader.LoadRecipe(ctx, &recipe)
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

func (e *engine) Delete(ctx context.Context, outputResources []rpv1.OutputResource, client processors.ResourceClient, recipe recipes.ResourceMetadata) error {
	definition, err := e.options.ConfigurationLoader.LoadRecipe(ctx, &recipe)
	if err != nil {
		return err
	}

	driver, ok := e.options.Drivers[definition.Driver]
	if !ok {
		return fmt.Errorf("could not find driver %s", definition.Driver)
	}

	return driver.Delete(ctx, outputResources, client)
}
