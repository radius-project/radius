// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package recipes

import (
	"fmt"
)

type RecipeMetadata struct {
	// Name represents the name of the recipe within the environment
	Name string
	// ApplicationID represents Fully qualified resource ID for the application that the link is linked to
	ApplicationID string
	// EnvironmentID represents Fully qualified resource ID for the application that the link is consumed by
	EnvironmentID string
	// ResourceID represents Fully qualified resource ID for the resource the recipe is deploying
	ResourceID string
	// Parameters represents Key/value parameters to pass into the recipe at deployment
	Parameters map[string]any
}

type ErrRecipeNotFound struct {
	Name        string
	Environment string
}

func (e *ErrRecipeNotFound) Error() string {
	return fmt.Sprintf("could not find recipe %q in environment %q", e.Name, e.Environment)
}
