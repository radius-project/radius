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
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// DaprStateStore represents DaprStateStore link resource.
type DaprStateStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprStateStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *DaprStateStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues
	if cn, ok := do.ComputedValues[renderers.ComponentNameKey].(string); ok {
		r.Properties.ComponentName = cn
	}
	return nil
}

// OutputResources returns the output resources array.
func (r *DaprStateStore) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *DaprStateStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// # Function Explanation
//
// ResourceTypeName returns a string representing the resource type name of the DaprStateStore instance.
func (daprStateStore *DaprStateStore) ResourceTypeName() string {
	return linkrp.DaprStateStoresResourceType
}

// Recipe returns the recipe information of the resource. Returns nil if recipe execution is disabled.
func (r *DaprStateStore) Recipe() *linkrp.LinkRecipe {
	if r.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &r.Properties.Recipe
}

// DaprStateStoreProperties represents the properties of DaprStateStore resource.
type DaprStateStoreProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties
	// Specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning linkrp.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
	Metadata             map[string]any              `json:"metadata,omitempty"`
	Recipe               linkrp.LinkRecipe           `json:"recipe,omitempty"`
	Resources            []*linkrp.ResourceReference `json:"resources,omitempty"`
	Type                 string                      `json:"type,omitempty"`
	Version              string                      `json:"version,omitempty"`
}
