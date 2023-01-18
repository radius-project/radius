// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/rp"
)

// LinkMetadata represents internal DataModel properties common to all link types.
type LinkMetadata struct {
	// ComputedValues map is any resource values that will be needed for more operations.
	// For example; database name to generate secrets for cosmos DB.
	ComputedValues map[string]any `json:"computedValues,omitempty"`

	// Stores action to retrieve secret values. For Azure, connectionstring is accessed through cosmos listConnectionString operation, if secrets are not provided as input
	SecretValues map[string]rp.SecretValueReference `json:"secretValues,omitempty"`

	RecipeData RecipeData `json:"recipeData,omitempty"`
}

type RecipeData struct {
	RecipeProperties

	Provider string

	// API version to use to perform operations on resources supported by the link.
	// For example for Azure resources, every service has different REST API version that must be specified in the request.
	APIVersion string

	// Resource ids of the resources deployed by the recipe
	Resources []string
}

type RecipeProperties struct {
	LinkRecipe
	LinkType     string
	TemplatePath string
}

// LinkRecipe is the recipe details used to automatically deploy underlying infrastructure for a link
type LinkRecipe struct {
	// Name of the recipe within the environment to use
	Name string `json:"name,omitempty"`
	// Parameters are key/value parameters to pass into the recipe at deployment
	Parameters map[string]any `json:"parameters,omitempty"`
}

// LinkMode specifies the mode used to deploy a link
type LinkMode string

const (
	LinkModeRecipe         LinkMode = "recipe"   // mode recipe for link deployment
	LinkModeResource       LinkMode = "resource" // mode resource for link deployment
	LinkModeValues         LinkMode = "values"   // mode values for link deployment
	RecipeContextParameter string   = "context"  // parameter context for recipe deployment
)

// RecipeContext is used to have the link, environment, application and runtime information to be used by recipe
// Recipe template authors can leverage the RecipeContext parameter to access properties to help them name/configure their infrastructure
// This allows the recipe template to generate names & properties that are unique and repeatable for the Link calling the recipe
type RecipeContext struct {
	Resource    Resource     `json:"resource,omitempty"`
	Application ResourceInfo `json:"application,omitempty"`
	Environment ResourceInfo `json:"environment,omitempty"`
	Runtime     Runtime      `json:"runtime,omitempty"`
}

// Resource contains the information about the  resource that is deployed using recipe
type Resource struct {
	Name string `json:"name,omitempty"`
	ID   string `json:"id,omitempty"`
	Type string `json:"type,omitempty"`
}

type ResourceInfo struct {
	Name string `json:"name,omitempty"`
	ID   string `json:"id,omitempty"`
}

type Runtime struct {
	Kubernetes Kubernetes `json:"kubernetes,omitempty"`
}

type Kubernetes struct {
	ApplicationNamespace string `json:"applicationNamespace,omitempty"`
	EnvironmentNamespace string `json:"EnvironmentNamespace,omitempty"`
}
