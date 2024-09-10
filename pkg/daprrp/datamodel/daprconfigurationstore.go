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
	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	"github.com/radius-project/radius/pkg/portableresources"
	pr_dm "github.com/radius-project/radius/pkg/portableresources/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

// DaprConfigurationStore represents Dapr configuration store portable resource.
type DaprConfigurationStore struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprConfigurationStoreProperties `json:"properties"`

	// ResourceMetadata represents internal DataModel properties common to all portable resource types.
	pr_dm.PortableResourceMetadata
}

// ApplyDeploymentOutput updates the DaprConfigurationStore resource with the DeploymentOutput values.
func (r *DaprConfigurationStore) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns the OutputResources from the Properties of the DaprConfigurationStore instance.
func (r *DaprConfigurationStore) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the Dapr ConfigurationStore resource i.e. application resources metadata.
func (r *DaprConfigurationStore) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ResourceTypeName returns a string representing the resource type.
func (r *DaprConfigurationStore) ResourceTypeName() string {
	return dapr_ctrl.DaprConfigurationStoresResourceType
}

// Recipe returns the recipe information of the resource. Returns nil if recipe execution is disabled.
func (r *DaprConfigurationStore) Recipe() *portableresources.ResourceRecipe {
	if r.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		return nil
	}
	return &r.Properties.Recipe
}

// DaprConfigurationStoreProperties represents the properties of Dapr ConfigurationStore resource.
type DaprConfigurationStoreProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties

	// ResourceProvisioning specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning portableresources.ResourceProvisioning `json:"resourceProvisioning,omitempty"`

	// Metadata of the Dapr Configuration store resource.
	Metadata map[string]*rpv1.DaprComponentMetadataValue `json:"metadata,omitempty"`

	// The recipe used to automatically deploy underlying infrastructure for the Dapr Configuration store  resource.
	Recipe portableresources.ResourceRecipe `json:"recipe,omitempty"`

	// List of the resource IDs that support the Dapr Configuration store Broker resource.
	Resources []*portableresources.ResourceReference `json:"resources,omitempty"`

	// Type of the Dapr  Configuration store resource.
	Type string `json:"type,omitempty"`

	// Version of the Dapr Pub/Sub Broker resource.
	Version string `json:"version,omitempty"`

	// Auth information for the Dapr Configuration Store resource, mainly secret store name.
	Auth *rpv1.DaprComponentAuth `json:"auth,omitempty"`
}
