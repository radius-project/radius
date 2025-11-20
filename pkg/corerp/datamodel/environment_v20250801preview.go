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

const EnvironmentResourceType_v20250801preview = "Radius.Core/environments"

// Environment_v20250801preview represents the new 2025-08-01-preview environment resource.
type Environment_v20250801preview struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties EnvironmentProperties_v20250801preview `json:"properties"`
}

// ResourceTypeName returns the resource type of the Environment instance.
func (e *Environment_v20250801preview) ResourceTypeName() string {
	return EnvironmentResourceType_v20250801preview
}

// EnvironmentProperties_v20250801preview represents the properties of the new environment schema.
type EnvironmentProperties_v20250801preview struct {
	// RecipePacks is the list of recipe pack resource IDs linked to this environment.
	RecipePacks []string `json:"recipePacks,omitempty"`

	// RecipeParameters contains recipe-specific parameters that apply to all resources of a given type.
	// The key is the resource type (e.g., "Radius.Compute/containers") and the value is a map of parameter names to values.
	RecipeParameters map[string]map[string]any `json:"recipeParameters,omitempty"`

	// Providers contains cloud provider configuration for the environment.
	Providers *Providers_v20250801preview `json:"providers,omitempty"`

	// Simulated indicates if this is a simulated environment.
	Simulated bool `json:"simulated,omitempty"`
}

// Providers_v20250801preview represents cloud provider configurations for the environment.
type Providers_v20250801preview struct {
	// Azure provider configuration
	Azure *ProvidersAzure_v20250801preview `json:"azure,omitempty"`

	// AWS provider configuration
	AWS *ProvidersAWS_v20250801preview `json:"aws,omitempty"`

	// Kubernetes provider configuration
	Kubernetes *ProvidersKubernetes_v20250801preview `json:"kubernetes,omitempty"`
}

// ProvidersAzure_v20250801preview represents the Azure provider configuration.
type ProvidersAzure_v20250801preview struct {
	// SubscriptionId is the Azure subscription ID hosting deployed resources.
	SubscriptionId string `json:"subscriptionId"`

	// ResourceGroupName is the optional resource group name.
	ResourceGroupName string `json:"resourceGroupName,omitempty"`

	// Identity contains external identity settings.
	Identity *rpv1.IdentitySettings `json:"identity,omitempty"`
}

// ProvidersKubernetes_v20250801preview represents the Kubernetes provider configuration.
type ProvidersKubernetes_v20250801preview struct {
	// Namespace is the Kubernetes namespace to deploy workloads into.
	Namespace string `json:"namespace"`
}

// ProvidersAWS_v20250801preview represents the AWS provider configuration.
type ProvidersAWS_v20250801preview struct {
	// Scope is the target scope for AWS resources to be deployed into.
	Scope string `json:"scope"`
}
