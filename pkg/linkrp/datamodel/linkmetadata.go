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

// RecipeProperties represents the information needed to deploy a recipe
type RecipeProperties struct {
	LinkRecipe                   // LinkRecipe is the recipe of the resource to be deployed
	LinkType      string         // LinkType represent the type of the link
	TemplatePath  string         // TemplatePath represent the recipe location
	EnvParameters map[string]any // EnvParameters represents the parameters set by the operator while linking the recipe to an environment
}

// LinkRecipe is the recipe details used to automatically deploy underlying infrastructure for a link
type LinkRecipe struct {
	// Name of the recipe within the environment to use
	Name string `json:"name,omitempty"`
	// Parameters are key/value parameters to pass into the recipe at deployment
	Parameters map[string]any `json:"parameters,omitempty"`
}

// LinkMode Specifies how to build the state store resource. Options are to build automatically via ‘recipe’ or ‘resource’, or build manually via ‘values’. Selection determines which set of fields to additionally require.
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
	ResourceInfo
	Type string `json:"type"`
}

type ResourceInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type Runtime struct {
	Kubernetes Kubernetes `json:"kubernetes,omitempty"`
}

type Kubernetes struct {
	Namespace            string `json:"namespace,omitempty"`            // This is set to the applicationNamespace when the Link is application-scoped, and set to the environmentNamespace when the Link is environment scoped
	EnvironmentNamespace string `json:"environmentNamespace,omitempty"` // This is set to environment namespace when a resource is application-scoped.
}
