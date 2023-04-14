// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package driver

import (
	"context"

	"github.com/project-radius/radius/pkg/recipes"
)

// Driver is an interface to implement recipe deployment.
type Driver interface {
	// Execute fetches the recipe contents and deploys the recipe and returns deployed resources, secrets and values.
	Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.Metadata, definition recipes.Definition) (*recipes.RecipeOutput, error)
}

// RecipeContext Recipe template authors can leverage the RecipeContext parameter to access Link properties to
// generate name and properties that are unique for the Link calling the recipe.
type RecipeContext struct {
	// Resource represents the resource information of the deploying recipe resource.
	Resource Resource `json:"resource,omitempty"`
	// Application represents environment resource information.
	Application ResourceInfo `json:"application,omitempty"`
	// Environment represents environment resource information.
	Environment ResourceInfo `json:"environment,omitempty"`
	// Runtime represents Kubernetes Runtime configuration.
	Runtime recipes.RuntimeConfiguration `json:"runtime,omitempty"`
}

// Resource contains the information needed to deploy a recipe.
// In the case the resource is a Link, it represents the Link's id, name and type.
type Resource struct {
	// ResourceInfo represents name and id of the resource
	ResourceInfo
	// Type represents the resource type, this will be a namespace/type combo. Ex. Applications.Core/Environment
	Type string `json:"type"`
}

// ResourceInfo represents name and id of the resource
type ResourceInfo struct {
	// Name represents the resource name.
	Name string `json:"name"`
	// ID represents fully qualified resource id.
	ID string `json:"id"`
}
