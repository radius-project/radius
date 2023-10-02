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

package frontend

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

const (
	testHeaderfile = "resource-request-headers.json"
	testAPIVersion = "2023-10-01-preview"
)

// TestResourceDataModel represents test resource.
type TestResourceDataModel struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties *TestResourceDataModelProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (r *TestResourceDataModel) ResourceTypeName() string {
	return "Applications.Core/resources"
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (c *TestResourceDataModel) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	c.Properties.Status.OutputResources = do.DeployedOutputResources
	return nil
}

// OutputResources returns the output resources array.
func (c *TestResourceDataModel) OutputResources() []rpv1.OutputResource {
	return c.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (h *TestResourceDataModel) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// TestResourceDataModelProperties represents the properties of TestResourceDataModel.
type TestResourceDataModelProperties struct {
	rpv1.BasicResourceProperties
	PropertyA string `json:"propertyA,omitempty"`
	PropertyB string `json:"propertyB,omitempty"`
}

// TestResource represents test resource for api version.
type TestResource struct {
	ID         *string                 `json:"id,omitempty"`
	Name       *string                 `json:"name,omitempty"`
	SystemData *v1.SystemData          `json:"systemData,omitempty"`
	Type       *string                 `json:"type,omitempty"`
	Location   *string                 `json:"location,omitempty"`
	Properties *TestResourceProperties `json:"properties,omitempty"`
	Tags       map[string]*string      `json:"tags,omitempty"`
}

// TestResourceProperties - HTTP Route properties
type TestResourceProperties struct {
	ProvisioningState *v1.ProvisioningState `json:"provisioningState,omitempty"`
	Environment       *string               `json:"environment,omitempty"`
	Application       *string               `json:"application,omitempty"`
	PropertyA         *string               `json:"propertyA,omitempty"`
	PropertyB         *string               `json:"propertyB,omitempty"`
	Status            *ResourceStatus       `json:"status,omitempty"`
}

// ResourceStatus - Status of a resource.
type ResourceStatus struct {
	OutputResources []map[string]any `json:"outputResources,omitempty"`
}

// ConvertTo converts a TestResource object to a TestResourceDataModel object and returns it.
func (src *TestResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &TestResourceDataModel{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      testAPIVersion,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: &TestResourceDataModelProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
			PropertyA: to.String(src.Properties.PropertyA),
			PropertyB: to.String(src.Properties.PropertyB),
		},
	}
	return converted, nil
}

// ConvertFrom converts a TestResourceDataModel into a TestResource, mapping fields from the DataModelInterface to the
// TestResource struct and converting the ProvisioningState from a DataModel to an internal type.
// It returns an error if the DataModelInterface is not a TestResourceDataModel.
func (dst *TestResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*TestResourceDataModel)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(dm.ID)
	dst.Name = to.Ptr(dm.Name)
	dst.Type = to.Ptr(dm.Type)
	dst.SystemData = &dm.SystemData
	dst.Location = to.Ptr(dm.Location)
	dst.Tags = *to.StringMapPtr(dm.Tags)
	dst.Properties = &TestResourceProperties{
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(dm.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(dm.InternalMetadata.AsyncProvisioningState),
		Environment:       to.Ptr(dm.Properties.Environment),
		Application:       to.Ptr(dm.Properties.Application),
		PropertyA:         to.Ptr(dm.Properties.PropertyA),
		PropertyB:         to.Ptr(dm.Properties.PropertyB),
	}

	return nil
}

func toProvisioningStateDataModel(state *v1.ProvisioningState) v1.ProvisioningState {
	if state == nil {
		return v1.ProvisioningStateAccepted
	}
	return *state
}

func fromProvisioningStateDataModel(state v1.ProvisioningState) *v1.ProvisioningState {
	converted := v1.ProvisioningStateAccepted
	if state != "" {
		converted = state
	}

	return &converted
}
