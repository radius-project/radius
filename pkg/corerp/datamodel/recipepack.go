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
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

const RecipePackResourceType = "Applications.Core/recipePacks"

// RecipePack represents the recipe pack resource.
type RecipePack struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RecipePackProperties `json:"properties"`
}

// ResourceTypeName returns the resource type of the RecipePack instance.
func (r *RecipePack) ResourceTypeName() string {
	return RecipePackResourceType
}

// ApplyDeploymentOutput updates the status of the recipe pack with the output resources from the deployment and returns no error.
// Since recipePacks are metadata resources, this is a no-op.
func (r *RecipePack) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns the OutputResources from the RecipePack instance.
// Since recipePacks are metadata resources, this returns empty slice.
func (r *RecipePack) OutputResources() []rpv1.OutputResource {
	return []rpv1.OutputResource{}
}

// ResourceMetadata returns an adapter that provides standardized access to BasicResourceProperties of the RecipePack resource.
func (r *RecipePack) ResourceMetadata() rpv1.BasicResourcePropertiesAdapter {
	return &r.Properties.BasicResourceProperties
}

// RecipePackProperties represents the properties of RecipePack.
type RecipePackProperties struct {
	rpv1.BasicResourceProperties

	// Description of what this recipe pack provides
	Description *string `json:"description,omitempty"`

	// Map of resource types to their recipe configurations
	Recipes map[string]RecipePackDefinition `json:"recipes"`
}

// RecipePackDefinition represents a recipe definition for a specific resource type in a recipe pack.
type RecipePackDefinition struct {
	// The type of recipe (e.g., terraform, bicep)
	RecipeKind string `json:"recipeKind"`

	// URL or path to the recipe source
	RecipeLocation string `json:"recipeLocation"`

	// Parameters to pass to the recipe
	Parameters map[string]any `json:"parameters,omitempty"`
}