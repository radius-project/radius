// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// Environment represents Application environment resource.
type Environment struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties EnvironmentProperties `json:"properties"`
}

func (e *Environment) ResourceTypeName() string {
	return "Applications.Core/environments"
}

// EnvironmentProperties represents the properties of Environment.
type EnvironmentProperties struct {
	Compute       rpv1.EnvironmentCompute                           `json:"compute,omitempty"`
	Recipes       map[string]map[string]EnvironmentRecipeProperties `json:"recipes,omitempty"`
	Providers     Providers                                         `json:"providers,omitempty"`
	UseDevRecipes bool                                              `json:"useDevRecipes,omitempty"`
	Extensions    []Extension                                       `json:"extensions,omitempty"`
}

// EnvironmentRecipeProperties represents the properties of environment's recipe.
type EnvironmentRecipeProperties struct {
	TemplatePath string         `json:"templatePath,omitempty"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}

// RecipeNameAndLinkType - Recipe Name and LinkType
type RecipeNameAndLinkType struct {
	// Type of the link this recipe can be consumed by. For example: 'Applications.Link/mongoDatabases'
	LinkType string `json:"linkType,omitempty"`

	// Name of the recipe registered to the environment.
	RecipeName string `json:"recipeName,omitempty"`
}

func (e *RecipeNameAndLinkType) ResourceTypeName() string {
	return "Applications.Core/environments"
}

func (e *EnvironmentRecipeProperties) ResourceTypeName() string {
	return "Applications.Core/environments"
}

// Providers represents configs for providers for the environment, eg azure,aws
type Providers struct {
	// Azure provider information
	Azure ProvidersAzure `json:"azure,omitempty"`
	// AWS provider information
	AWS ProvidersAWS `json:"aws,omitempty"`
}

// ProvidersAzure represents the azure provider configs
type ProvidersAzure struct {
	// Scope is the target level for deploying the azure resources
	Scope string `json:"scope,omitempty"`
}

// ProvidersAWS represents the aws provider configs
type ProvidersAWS struct {
	// Scope is the target level for deploying the aws resources
	Scope string `json:"scope,omitempty"`
}
