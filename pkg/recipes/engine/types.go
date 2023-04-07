// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package engine

import (
	"context"

	"github.com/project-radius/radius/pkg/recipes"
)

type Engine interface {
	// Execute gathers environment configuration and recipe definition and calls the driver to deploy the recipe.
	Execute(ctx context.Context, recipe recipes.Metadata) (*recipes.RecipeOutput, error)
}
