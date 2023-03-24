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
	Execute(ctx context.Context, recipe recipes.RecipeMetadata) (*recipes.RecipeResult, error)
}
