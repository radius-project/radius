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
	linkrpdm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// DaprStateStore represents DaprStateStore link resource.
type DaprStateStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprStateStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	linkrpdm.LinkMetadata
}

// # Function Explanation
//
// DaprStateStore.ApplyDeploymentOutput updates the status, computed values, secret values and component name of the
// DaprStateStore instance and returns no error.
func (r *DaprStateStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues
	if cn, ok := do.ComputedValues[renderers.ComponentNameKey].(string); ok {
		r.Properties.ComponentName = cn
	}
	return nil
}

// # Function Explanation
//
// Method OutputResources returns the OutputResources from the Properties field of the DaprStateStore instance.
func (r *DaprStateStore) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// # Function Explanation
//
// ResourceMetadata returns the BasicResourceProperties of the DaprStateStore instance.
func (r *DaprStateStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// # Function Explanation
//
// ResourceTypeName returns the resource type of the DaprStateStore instance.
func (daprStateStore *DaprStateStore) ResourceTypeName() string {
	return linkrp.N_DaprStateStoresResourceType
}

// # Function Explanation
//
// Recipe returns a pointer to the LinkRecipe stored in the Properties of the DaprStateStore instance.
func (r *DaprStateStore) Recipe() *linkrp.LinkRecipe {
	return &r.Properties.Recipe
}

// DaprStateStoreProperties represents the properties of DaprStateStore resource.
type DaprStateStoreProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties
	Mode     linkrpdm.LinkMode `json:"mode,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
	Recipe   linkrp.LinkRecipe `json:"recipe,omitempty"`
	Resource string            `json:"resource,omitempty"`
	Type     string            `json:"type,omitempty"`
	Version  string            `json:"version,omitempty"`
}
