// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package driver

import (
	"context"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
)

type Driver interface {
	Execute(ctx context.Context, configuration configloader.Configuration, recipe recipes.RecipeMetadata, definition configloader.RecipeDefinition) (*recipes.RecipeResponse, error)
}

// RecipeContext Recipe template authors can leverage the RecipeContext parameter to access Link properties to
// generate name and properties that are unique for the Link calling the recipe.
type RecipeContext struct {
	Resource    Resource                          `json:"resource,omitempty"`
	Application ResourceInfo                      `json:"application,omitempty"`
	Environment ResourceInfo                      `json:"environment,omitempty"`
	Runtime     configloader.RuntimeConfiguration `json:"runtime,omitempty"`
}

// Resource contains the information needed to deploy a recipe.
// In the case the resource is a Link, it represents the Link's id, name and type.
type Resource struct {
	ResourceInfo
	Type string `json:"type"`
}

// ResourceInfo name and id of the resource
type ResourceInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}
