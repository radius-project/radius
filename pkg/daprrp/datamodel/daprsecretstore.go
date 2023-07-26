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
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// DaprSecretStore represents DaprSecretStore link resource.
type DaprSecretStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprSecretStoreProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	linkrpdm.LinkMetadata
}

// # Function Explanation
//
// ApplyDeploymentOutput updates the status of the secret store with the output resources from a deployment.
func (r *DaprSecretStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// # Function Explanation
//
// OutputResources returns the OutputResources from the Properties field of the DaprSecretStore instance.
func (r *DaprSecretStore) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// # Function Explanation
//
// ResourceMetadata returns the BasicResourceProperties of the DaprSecretStore instance.
func (r *DaprSecretStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// # Function Explanation
//
// ResourceTypeName returns the resource type of the DaprSecretStore instance.
func (daprSecretStore *DaprSecretStore) ResourceTypeName() string {
	return linkrp.N_DaprSecretStoresResourceType
}

// DaprSecretStoreProperties represents the properties of DaprSecretStore resource.
type DaprSecretStoreProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties
	Mode     linkrpdm.LinkMode `json:"mode"`
	Type     string            `json:"type"`
	Version  string            `json:"version"`
	Metadata map[string]any    `json:"metadata"`
	Recipe   linkrp.LinkRecipe `json:"recipe,omitempty"`
}
