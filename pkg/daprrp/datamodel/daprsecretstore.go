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
	linkrp_dm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// DaprSecretStore represents Dapr SecretStore portable resource.
type DaprSecretStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprSecretStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all portable resource types.
	linkrp_dm.LinkMetadata
}

// # Function Explanation
//
// ApplyDeploymentOutput updates the Status.OutputResources field of Properties with the DeployedOutputResources
// from the DeploymentOutput and returns no error.
func (r *DaprSecretStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// # Function Explanation
//
// OutputResources returns the OutputResources array from Properties of the Dapr SecretStore resource.
func (r *DaprSecretStore) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// # Function Explanation
//
// ResourceMetadata returns the BasicResourceProperties of the DaprSecretStore resource i.e. application resources metadata.
func (r *DaprSecretStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// # Function Explanation
//
// ResourceTypeName returns the resource type of the DaprSecretStore resource.
func (daprSecretStore *DaprSecretStore) ResourceTypeName() string {
	return linkrp.N_DaprSecretStoresResourceType
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

// # Function Explanation
//
// Recipe returns the Recipe from the DaprSecretStore Properties if ResourceProvisioning is not set to Manual,
// otherwise it returns nil.
func (daprSecretStore *DaprSecretStore) Recipe() *linkrp.LinkRecipe {
	if daprSecretStore.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &daprSecretStore.Properties.Recipe
}
