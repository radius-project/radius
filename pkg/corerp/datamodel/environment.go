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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

const EnvironmentResourceType = "Applications.Core/environments"

// Environment represents Application environment resource.
type Environment struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties EnvironmentProperties `json:"properties"`
}

// ResourceTypeName returns the resource type of the Environment instance.
func (e *Environment) ResourceTypeName() string {
	return EnvironmentResourceType
}

// EnvironmentProperties represents the properties of Environment.
type EnvironmentProperties struct {
	Compute    rpv1.EnvironmentCompute                           `json:"compute,omitempty"`
	Recipes    map[string]map[string]EnvironmentRecipeProperties `json:"recipes,omitempty"`
	Providers  Providers                                         `json:"providers,omitempty"`
	Extensions []Extension                                       `json:"extensions,omitempty"`
}

// EnvironmentRecipeProperties represents the properties of environment's recipe.
type EnvironmentRecipeProperties struct {
	TemplateKind    string         `json:"templateKind"`
	TemplatePath    string         `json:"templatePath"`
	TemplateVersion string         `json:"templateVersion,omitempty"`
	Parameters      map[string]any `json:"parameters,omitempty"`
}

// Recipe represents input properties for recipe getMetadata api.
type Recipe struct {
	// Type of the link this recipe can be consumed by. For example: 'Applications.Link/mongoDatabases'
	LinkType string `json:"linkType,omitempty"`

	// Name of the recipe registered to the environment.
	Name string `json:"recipeName,omitempty"`
}

// ResourceTypeName returns the resource type of the Recipe instance.
func (e *Recipe) ResourceTypeName() string {
	return "Applications.Core/environments"
}

// ResourceTypeName returns the resource type of the EnvironmentRecipeProperties instance.
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
