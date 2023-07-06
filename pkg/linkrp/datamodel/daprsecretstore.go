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
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// DaprSecretStore represents DaprSecretStore link resource.
type DaprSecretStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprSecretStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *DaprSecretStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (r *DaprSecretStore) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
//
// # Function Explanation
//
// ResourceMetadata returns the BasicResourceProperties of the DaprSecretStore instance.
func (r *DaprSecretStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// # Function Explanation
//
// ResourceTypeName returns the resource type name for the DaprSecretStore instance.
func (daprSecretStore *DaprSecretStore) ResourceTypeName() string {
	return linkrp.DaprSecretStoresResourceType
}

// DaprSecretStoreProperties represents the properties of DaprSecretStore resource.
type DaprSecretStoreProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties
	Type                 string                      `json:"type,omitempty"`
	Version              string                      `json:"version,omitempty"`
	Metadata             map[string]any              `json:"metadata,omitempty"`
	Recipe               linkrp.LinkRecipe           `json:"recipe,omitempty"`
	ResourceProvisioning linkrp.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
}

// Recipe returns the recipe for the Dapr Secret Store
func (daprSecretStore *DaprSecretStore) Recipe() *linkrp.LinkRecipe {
	if daprSecretStore.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &daprSecretStore.Properties.Recipe
}
