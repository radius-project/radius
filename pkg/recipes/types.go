// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package recipes

import (
	"fmt"
)

type RecipeContext struct {
	//Recipe name
	Name string
	//Application ID
	ApplicationID string
	//Environment ID
	EnvironmentID string
	//TrackedResource ID
	ResourceID string
	//Recipe parameters
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
