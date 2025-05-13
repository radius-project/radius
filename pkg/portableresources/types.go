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

package portableresources

import (
	"github.com/radius-project/radius/pkg/recipes/util"
)

const (
	// ResourceProvisioningRecipe is the scenario when Radius manages the lifecycle of the resource through a Recipe.
	ResourceProvisioningRecipe ResourceProvisioning = "recipe"

	// ResourceProvisioningManual is the scenario where the user manages the resource and provides values.
	ResourceProvisioningManual ResourceProvisioning = "manual"

	// DefaultRecipeName represents the default recipe name.
	DefaultRecipeName = "default"
)

type RecipeData struct {
	RecipeProperties

	// APIVersion is the API version to use to perform operations on resources.
	// For example for Azure resources, every service has different REST API version that must be specified in the request.
	APIVersion string

	// Resource ids of the resources deployed by the recipe
	Resources []string
}

// RecipeProperties represents the information needed to deploy a recipe
type RecipeProperties struct {
	ResourceRecipe                // ResourceRecipe is the recipe of the resource to be deployed
	ResourceType   string         // ResourceType represent the type of the resource
	TemplatePath   string         // TemplatePath represent the recipe location
	EnvParameters  map[string]any // EnvParameters represents the parameters set by the operator while linking the recipe to an environment
}

// ResourceRecipe is the recipe details used to automatically deploy underlying infrastructure for a resource.
type ResourceRecipe struct {
	// Name of the recipe within the environment to use
	Name string `json:"name,omitempty"`
	// Parameters are key/value parameters to pass into the recipe at deployment
	Parameters map[string]any `json:"parameters,omitempty"`
	// DeploymentStatus is the deployment status of the recipe
	DeploymentStatus util.RecipeDeploymentStatus `json:"recipeStatus,omitempty"`
}

// ResourceReference represents a reference to a resource that was deployed by the user
// and specified as part of a portable resource.
//
// This type should be used in datamodels for the '.properties.resources' field.
type ResourceReference struct {
	ID string `json:"id"`
}

// ResourceProvisioning specifies how the resource should be managed
type ResourceProvisioning string
