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

package v20231001preview

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// ConvertTo converts from the versioned LocationResource resource to version-agnostic datamodel.
func (src *LocationResource) ConvertTo() (v1.DataModelInterface, error) {
	dst := &datamodel.Location{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   to.String(src.ID),
				Name: to.String(src.Name),
				Type: datamodel.LocationResourceType,

				// NOTE: this is a child resource. It does not have a location, systemData, or tags.
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},
	}

	dst.Properties = datamodel.LocationProperties{
		Address:       src.Properties.Address,
		ResourceTypes: map[string]datamodel.LocationResourceTypeConfiguration{},
	}

	for name, value := range src.Properties.ResourceTypes {
		dst.Properties.ResourceTypes[name] = toLocationResourceTypeDatamodel(value)
	}

	return dst, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned LocationResource resource.
func (dst *LocationResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.Location)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(dm.ID)
	dst.Name = to.Ptr(dm.Name)
	dst.Type = to.Ptr(datamodel.LocationResourceType)

	// NOTE: this is a child resource. It does not have a location, systemData, or tags.

	dst.Properties = &LocationProperties{
		ProvisioningState: to.Ptr(ProvisioningState(dm.InternalMetadata.AsyncProvisioningState)),
		Address:           dm.Properties.Address,
		ResourceTypes:     map[string]*LocationResourceType{},
	}

	for name, value := range dm.Properties.ResourceTypes {
		dst.Properties.ResourceTypes[name] = fromLocationResourceTypeDatamodel(value)
	}

	return nil
}

func toLocationResourceTypeDatamodel(src *LocationResourceType) datamodel.LocationResourceTypeConfiguration {
	dst := datamodel.LocationResourceTypeConfiguration{
		APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{},
	}

	for name, value := range src.APIVersions {
		dst.APIVersions[name] = toLocationAPIVersionDatamodel(value)
	}

	return dst
}

func toLocationAPIVersionDatamodel(_ map[string]any) datamodel.LocationAPIVersionConfiguration {
	dst := datamodel.LocationAPIVersionConfiguration{
		// Empty for now.
	}
	return dst
}

func fromLocationResourceTypeDatamodel(src datamodel.LocationResourceTypeConfiguration) *LocationResourceType {
	dst := &LocationResourceType{
		APIVersions: map[string]map[string]any{},
	}

	for name, value := range src.APIVersions {
		dst.APIVersions[name] = fromLocationAPIVersionDatamodel(value)
	}

	return dst
}

func fromLocationAPIVersionDatamodel(src datamodel.LocationAPIVersionConfiguration) map[string]any {
	dst := map[string]any{
		// Empty for now.
	}
	return dst
}
