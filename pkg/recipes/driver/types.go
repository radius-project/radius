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

package driver

import (
	"context"

	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// Driver is an interface to implement recipe deployment.
type Driver interface {
	// Execute fetches the recipe contents and deploys the recipe and returns deployed resources, secrets and values.
	Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.ResourceMetadata, definition recipes.EnvironmentDefinition) (*recipes.RecipeOutput, error)
	// Delete handles deletion of output resources for the recipe deployment.
	Delete(ctx context.Context, deploymentDataModel rpv1.DeploymentDataModel, client processors.ResourceClient) error
}

// RecipeContext Recipe template authors can leverage the RecipeContext parameter to access Link properties to
// generate name and properties that are unique for the Link calling the recipe.
type RecipeContext struct {
	// Resource represents the resource information of the deploying recipe resource.
	Resource Resource `json:"resource,omitempty"`
	// Application represents environment resource information.
	Application ResourceInfo `json:"application,omitempty"`
	// Environment represents environment resource information.
	Environment ResourceInfo `json:"environment,omitempty"`
	// Runtime represents Kubernetes Runtime configuration.
	Runtime recipes.RuntimeConfiguration `json:"runtime,omitempty"`
}

// Resource contains the information needed to deploy a recipe.
// In the case the resource is a Link, it represents the Link's id, name and type.
type Resource struct {
	// ResourceInfo represents name and id of the resource
	ResourceInfo
	// Type represents the resource type, this will be a namespace/type combo. Ex. Applications.Core/Environment
	Type string `json:"type"`
}

// ResourceInfo represents name and id of the resource
type ResourceInfo struct {
	// Name represents the resource name.
	Name string `json:"name"`
	// ID represents fully qualified resource id.
	ID string `json:"id"`
}
