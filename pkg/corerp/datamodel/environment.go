// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

// EnvironmentComputeKind is the type of compute resource.
type EnvironmentComputeKind string

const (
	// UnknownComputeKind represents kubernetes compute resource type.
	UnknownComputeKind EnvironmentComputeKind = "unknown"
	// KubernetesComputeKind represents kubernetes compute resource type.
	KubernetesComputeKind EnvironmentComputeKind = "kubernetes"
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
	Compute       EnvironmentCompute                     `json:"compute,omitempty"`
	Recipes       map[string]EnvironmentRecipeProperties `json:"recipes,omitempty"`
	Providers     Providers                              `json:"providers,omitempty"`
	UseDevRecipes bool                                   `json:"useDevRecipes,omitempty"`
	Extensions    []Extension                            `json:"extensions,omitempty"`
}

// EnvironmentCompute represents the compute resource of Environment.
type EnvironmentCompute struct {
	Kind              EnvironmentComputeKind      `json:"kind"`
	KubernetesCompute KubernetesComputeProperties `json:"kubernetes,omitempty"`

	// Environment-level identity that can be used by any resource in the environment.
	// Resources can specify its own identities and they will override the environment-level identity.
	Identity *rp.IdentitySettings `json:"identity"`
}

// KubernetesComputeProperties represents the kubernetes compute of the environment.
type KubernetesComputeProperties struct {
	ResourceID string `json:"resourceId,omitempty"`
	Namespace  string `json:"namespace"`
}

// EnvironmentRecipeProperties represents the properties of environment's recipe.
type EnvironmentRecipeProperties struct {
	LinkType     string `json:"linkType,omitempty"`
	TemplatePath string `json:"templatePath,omitempty"`
}

// Providers represents configs for providers for the environment, eg azure
type Providers struct {
	Azure ProvidersAzure `json:"azure,omitempty"`
}

// ProvidersAzure represents the azure provider configs
type ProvidersAzure struct {
	Scope string `json:"scope,omitempty"`
}

// EnvironmentKubernetesMetadataExtension - Specifies user defined labels and annotations
type EnvironmentKubernetesMetadataExtension struct {
	BaseKubernetesMetadataExtension
}
