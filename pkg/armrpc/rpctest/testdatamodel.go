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

package rpctest

import (
	"encoding/json"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
)

const (
	TestAPIVersion = "2022-03-15-privatepreview"
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

// TestResourceDataModelProperties represents the properties of TestResourceDataModel.
type TestResourceDataModelProperties struct {
	Application string `json:"application"`
	Environment string `json:"environment"`
	PropertyA   string `json:"propertyA,omitempty"`
	PropertyB   string `json:"propertyB,omitempty"`
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
}

// # Function Explanation
//
// ConvertTo converts a version specific TestResource into a version-agnostic resource, TestResourceDataModel.
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
				UpdatedAPIVersion:      TestAPIVersion,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: &TestResourceDataModelProperties{
			Application: to.String(src.Properties.Application),
			Environment: to.String(src.Properties.Environment),
			PropertyA:   to.String(src.Properties.PropertyA),
			PropertyB:   to.String(src.Properties.PropertyB),
		},
	}
	return converted, nil
}

// # Function Explanation
//
// ConvertFrom converts src version agnostic model to versioned model, TestResource.
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

func TestResourceDataModelToVersioned(model *TestResourceDataModel, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case TestAPIVersion:
		versioned := &TestResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

func TestResourceDataModelFromVersioned(content []byte, version string) (*TestResourceDataModel, error) {
	switch version {
	case TestAPIVersion:
		am := &TestResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*TestResourceDataModel), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
