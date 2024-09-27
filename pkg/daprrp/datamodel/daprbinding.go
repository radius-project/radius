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
	"github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

// DaprBinding represents Dapr binding portable resource.
type DaprBinding struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties DaprBindingProperties `json:"properties"`

	// ResourceMetadata represents internal DataModel properties common to all portable resource types.
	datamodel.PortableResourceMetadata
}

// ApplyDeploymentOutput updates the DaprBinding resource with the DeploymentOutput values.
func (r *DaprBinding) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns the OutputResources from the Properties of the DaprBinding instance.
func (r *DaprBinding) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the Dapr Binding resource i.e. application resources metadata.
func (r *DaprBinding) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ResourceTypeName returns a string representing the resource type.
func (r *DaprBinding) ResourceTypeName() string {
	return controller.DaprBindingsResourceType
}

// Recipe returns the recipe information of the resource. Returns nil if recipe execution is disabled.
func (r *DaprBinding) Recipe() *portableresources.ResourceRecipe {
	if r.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		return nil
	}
	return &r.Properties.Recipe
}

// DaprBindingProperties represents the properties of Dapr Binding resource.
type DaprBindingProperties struct {
	rpv1.BasicResourceProperties
	rpv1.BasicDaprResourceProperties

	// ResourceProvisioning specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning portableresources.ResourceProvisioning `json:"resourceProvisioning,omitempty"`

	// Metadata of the Dapr Binding resource.
	Metadata map[string]*rpv1.DaprComponentMetadataValue `json:"metadata,omitempty"`

	// The recipe used to automatically deploy underlying infrastructure for the Dapr Binding resource.
	Recipe portableresources.ResourceRecipe `json:"recipe,omitempty"`

	// List of the resource IDs that support the Dapr Binding Broker resource.
	Resources []*portableresources.ResourceReference `json:"resources,omitempty"`

	// Type of the Dapr Binding resource.
	Type string `json:"type,omitempty"`

	// Version of the Dapr Binding resource.
	Version string `json:"version,omitempty"`

	// Auth information for the Dapr Binding resource, mainly secret store name.
	Auth *rpv1.DaprComponentAuth `json:"auth,omitempty"`

	// The list of Dapr app-IDs this component applies to. It applies to all apps when no scopes are specified.
	Scopes []string `json:"scopes,omitempty"`
}
