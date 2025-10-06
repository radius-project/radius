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

package datamodel

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

const RecipePackResourceType = "Radius.Core/recipePacks"

// RecipePack represents the 2025-08-01-preview recipe pack resource.
type RecipePack struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RecipePackProperties `json:"properties"`
}

// ResourceTypeName returns the resource type of the RecipePack instance.
func (r *RecipePack) ResourceTypeName() string {
	return RecipePackResourceType
}

// RecipePackProperties represents the properties of the recipe pack resource.
type RecipePackProperties struct {
	// Recipes is a map of resource types to their recipe configurations.
	Recipes map[string]*RecipeDefinition `json:"recipes"`

	// Description of what this recipe pack provides.
	Description string `json:"description,omitempty"`

	// ReferencedBy is a list of environment IDs that reference this recipe pack.
	ReferencedBy []string `json:"referencedBy,omitempty"`
}

// RecipeDefinition represents a recipe definition in the datamodel.
type RecipeDefinition struct {
	// RecipeKind is the type of recipe (e.g., terraform, bicep).
	RecipeKind string `json:"recipeKind"`

	// RecipeLocation is the URL or path to the recipe source.
	RecipeLocation string `json:"recipeLocation"`

	// Parameters to pass to the recipe.
	Parameters any `json:"parameters,omitempty"`

	// PlainHTTP connects to the location using HTTP (not-HTTPS).
	PlainHTTP bool `json:"plainHTTP,omitempty"`
}
