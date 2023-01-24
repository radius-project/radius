// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
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
	Compute       rp.EnvironmentCompute                  `json:"compute,omitempty"`
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

// Providers represents configs for providers for the environment, eg azure,aws
type Providers struct {
	Azure ProvidersAzure `json:"azure,omitempty"` // Azure provider information
	AWS   ProvidersAWS   `json:"aws,omitempty"`   // AWS provider information
}

// ProvidersAzure represents the azure provider configs
type ProvidersAzure struct {
	Scope string `json:"scope,omitempty"` // scope for deploying the azure resources
}

// ProvidersAWS represents the aws provider configs
type ProvidersAWS struct {
	Scope string `json:"scope,omitempty"` // scope for deploying the aws resources
}
