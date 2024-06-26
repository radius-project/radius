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
	"github.com/radius-project/radius/pkg/portableresources"
	portableresourcesdm "github.com/radius-project/radius/pkg/portableresources/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

var _ v1.ResourceDataModel = (*DynamicResource)(nil)
var _ rpv1.RadiusResourceModel = (*DynamicResource)(nil)
var _ portableresourcesdm.RecipeDataModel = (*DynamicResource)(nil)

// DynamicResource is used as the data model for dynamic resources.
//
// A dynamic resource is implemented internally to UCP, and uses a user-provided
// OpenAPI specification to define the resource schema. Since the resource is internal
// to UCP and dynamically generated, this struct is used to represent all dynamic resources.
type DynamicResource struct {
	v1.BaseResource

	// Properties stores the properties of the resource being tracked.
	Properties map[string]any `json:"properties"`
}

type DynamicResourceStatus struct {
	rpv1.ResourceStatus

	Binding map[string]any `json:"binding,omitempty"`
}

// ResourceTypeName gives the type of the resource.
func (r *DynamicResource) ResourceTypeName() string {
	return r.Type
}

func (r *DynamicResource) Status() DynamicResourceStatus {
	if r.Properties == nil {
		r.Properties = map[string]any{}
	}

	obj := r.Properties["status"]
	if obj == nil {
		r.Properties["status"] = &DynamicResourceStatus{}
	} else if _, ok := obj.(*DynamicResourceStatus); !ok {
		r.Properties["status"] = &DynamicResourceStatus{}
	}

	return *(r.Properties["status"].(*DynamicResourceStatus))
}

func (r *DynamicResource) SetStatus(status DynamicResourceStatus) {
	if r.Properties == nil {
		r.Properties = map[string]any{}
	}

	r.Properties["status"] = &status
}

// ApplyDeploymentOutput is a method required by the RadiusResourceModel interface.
func (r *DynamicResource) ApplyDeploymentOutput(output rpv1.DeploymentOutput) error {
	status := r.Status()
	status.OutputResources = output.DeployedOutputResources

	status.Binding = map[string]any{}
	for key, value := range output.ComputedValues {
		status.Binding[key] = value
	}

	for key, reference := range output.SecretValues {
		status.Binding[key] = reference.Value
	}

	r.SetStatus(status)

	return nil
}

func (r *DynamicResource) OutputResources() []rpv1.OutputResource {
	return r.ResourceMetadata().Status.OutputResources
}

func (r *DynamicResource) ResourceMetadata() *rpv1.BasicResourceProperties {
	if r.Properties == nil {
		r.Properties = map[string]any{}
	}

	status := r.Status()

	application := ""
	obj := r.Properties["application"]
	if obj != nil {
		if s, ok := obj.(string); ok {
			application = s
		}
	}

	environment := ""
	obj = r.Properties["environment"]
	if obj != nil {
		if s, ok := obj.(string); ok {
			environment = s
		}
	}

	return &rpv1.BasicResourceProperties{Application: application, Environment: environment, Status: status.ResourceStatus}
}

// Recipe provides access to the user-specified recipe configuration.
func (r *DynamicResource) Recipe() *portableresources.ResourceRecipe {
	if r.Properties == nil {
		r.Properties = map[string]any{}
	}

	obj := r.Properties["recipe"]
	if obj == nil {
		r.Properties["recipe"] = &portableresources.ResourceRecipe{}
	} else if _, ok := obj.(*portableresources.ResourceRecipe); !ok {
		r.Properties["recipe"] = &portableresources.ResourceRecipe{}
	}

	recipe := r.Properties["recipe"].(*portableresources.ResourceRecipe)
	if recipe.Name == "" {
		recipe.Name = "default"
	}

	return recipe
}

func (r *DynamicResource) SetRecipeStatus(status rpv1.RecipeStatus) {
	s := r.Status()
	s.Recipe = &status
	r.SetStatus(s)
}
