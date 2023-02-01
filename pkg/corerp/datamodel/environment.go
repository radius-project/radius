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
	Compute       rpv1.EnvironmentCompute                `json:"compute,omitempty"`
	Recipes       map[string]EnvironmentRecipeProperties `json:"recipes,omitempty"`
	Providers     Providers                              `json:"providers,omitempty"`
	UseDevRecipes bool                                   `json:"useDevRecipes,omitempty"`
	Extensions    []Extension                            `json:"extensions,omitempty"`
}

// EnvironmentRecipeProperties represents the properties of environment's recipe.
type EnvironmentRecipeProperties struct {
	LinkType     string         `json:"linkType,omitempty"`
	TemplatePath string         `json:"templatePath,omitempty"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}

func (e *EnvironmentRecipeProperties) ResourceTypeName() string {
	return "Applications.Core/environmentrecipeproperties"
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
