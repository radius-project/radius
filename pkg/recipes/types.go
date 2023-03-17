// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package recipes

import (
	"fmt"
)

type RecipeMetadata struct {
	//The name of the recipe within the environment
	Name string
	//Fully qualified resource ID for the application that the link is linked to
	ApplicationID string
	//Fully qualified resource ID for the application that the link is consumed by
	EnvironmentID string
	//Fully qualified resource ID for the resource the recipe is deploying
	ResourceID string
	//Key/value parameters to pass into the recipe at deployment
	Parameters map[string]any
}

type ErrRecipeNotFound struct {
	Name        string
	Environment string
}

func (e *ErrRecipeNotFound) Error() string {
	return fmt.Sprintf("could not find recipe %q in environment %q", e.Name, e.Environment)
}

func (e *ErrRecipeNotFound) Is(other error) bool {
	_, ok := other.(*ErrRecipeNotFound)
	return ok
}
