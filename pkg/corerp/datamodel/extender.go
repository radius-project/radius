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

// ExtenderResourceType is the resource type for Extender link resources.
const ExtenderResourceType = "Applications.Core/extenders"

// Extender represents Extender link resource.
type Extender struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties ExtenderProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

// ApplyDeploymentOutput updates the Status of Properties of the Extender resource with the DeployedOutputResources and returns no error.
func (r *Extender) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the OutputResources of the Extender resource.
func (r *Extender) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the Extender resource.
func (r *Extender) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ResourceTypeName returns the resource type of the extender resource.
func (extender *Extender) ResourceTypeName() string {
	return ExtenderResourceType
}

// Recipe returns the LinkRecipe associated with the Extender if the ResourceProvisioning is not set to Manual,
// otherwise it returns nil.
func (extender *Extender) Recipe() *linkrp.LinkRecipe {
	if extender.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &extender.Properties.ResourceRecipe
}

// ExtenderProperties represents the properties of Extender resource.
type ExtenderProperties struct {
	rpv1.BasicResourceProperties
	// Additional properties for the resource
	AdditionalProperties map[string]any `json:"additionalProperties,omitempty"`
	// Secrets values provided for the resource
	Secrets map[string]any `json:"secrets,omitempty"`
	// The recipe used to automatically deploy underlying infrastructure for the Extender
	ResourceRecipe linkrp.LinkRecipe `json:"recipe,omitempty"`
	// Specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning linkrp.ResourceProvisioning `json:"resourceProvisioning,omitempty"`
}
