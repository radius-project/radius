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

// DaprPubSubBroker represents Dapr PubSubBroker portable resource.
type DaprPubSubBroker struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprPubSubBrokerProperties `json:"properties"`

	// ResourceMetadata represents internal DataModel properties common to all portable resource types.
	pr_dm.PortableResourceMetadata
}

// ApplyDeploymentOutput updates the DaprPubSubBroker resource with the DeploymentOutput values.
func (r *DaprPubSubBroker) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns the OutputResources from the Properties of the DaprPubSubBroker instance.
func (r *DaprPubSubBroker) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the Dapr PubSubBroker resource i.e. application resources metadata.
func (r *DaprPubSubBroker) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ResourceTypeName returns a string representing the resource type.
func (r *DaprPubSubBroker) ResourceTypeName() string {
	return dapr_ctrl.DaprPubSubBrokersResourceType
}

// Recipe returns the recipe information of the resource. Returns nil if recipe execution is disabled.
func (r *DaprPubSubBroker) Recipe() *portableresources.ResourceRecipe {
	if r.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		return nil
	}
	return &r.Properties.Recipe
}

// DaprPubSubBrokerProperties represents the properties of Dapr PubSubBroker resource.
type DaprPubSubBrokerProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties

	// Specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning portableresources.ResourceProvisioning `json:"resourceProvisioning,omitempty"`

	// Metadata of the Dapr Pub/Sub Broker resource.
	Metadata map[string]any `json:"metadata,omitempty"`

	// The recipe used to automatically deploy underlying infrastructure for the Dapr Pub/Sub Broker resource.
	Recipe portableresources.ResourceRecipe `json:"recipe,omitempty"`

	// List of the resource IDs that support the Dapr Pub/Sub Broker resource.
	Resources []*portableresources.ResourceReference `json:"resources,omitempty"`

	// Type of the Dapr Pub/Sub Broker resource.
	Type string `json:"type,omitempty"`

	// Version of the Dapr Pub/Sub Broker resource.
	Version string `json:"version,omitempty"`
}
