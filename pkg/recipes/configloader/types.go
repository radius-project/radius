// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"context"

	"github.com/project-radius/radius/pkg/recipes"
)

type ConfigurationLoader interface {
	// LoadConfiguration fetches environment/application information and return runtime and provider configuration.
	LoadConfiguration(ctx context.Context, recipe recipes.Metadata) (*recipes.Configuration, error)
	//	LoadRecipe fetches the recipe information from the environment.
	LoadRecipe(ctx context.Context, recipe recipes.Metadata) (*recipes.Definition, error)
}
