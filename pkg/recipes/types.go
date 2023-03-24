// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package recipes

import (
	"fmt"
)

// RecipeMetadata represents recipe details provided while creating a Link resource.
type RecipeMetadata struct {
	// Name represents the name of the recipe within the environment
	Name string
	// ApplicationID represents fully qualified resource ID for the application that the link is consumed by
	ApplicationID string
	// EnvironmentID represents fully qualified resource ID for the environment that the link is linked to
	EnvironmentID string
	// ResourceID represents fully qualified resource ID for the resource the recipe is deploying
	ResourceID string
	// Parameters represents Key/value parameters to pass into the recipe at deployment
	Parameters map[string]any
}

type RecipeResult struct {
	Resources []string
	Secrets   map[string]any
	Values    map[string]any
}

type ErrRecipeNotFound struct {
	Name        string
	Environment string
}

func (e *ErrRecipeNotFound) Error() string {
	return fmt.Sprintf("could not find recipe %q in environment %q", e.Name, e.Environment)
}
