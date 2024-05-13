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

// ConvertTo converts from the versioned Radius Plane resource to version-agnostic datamodel.
func (src *RadiusPlaneResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.RadiusPlane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},

		Properties: datamodel.RadiusPlaneProperties{
			ResourceProviders: to.StringMap(src.Properties.ResourceProviders),
		},
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Radius Plane resource.
func (dst *RadiusPlaneResource) ConvertFrom(src v1.DataModelInterface) error {
	plane, ok := src.(*datamodel.RadiusPlane)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &plane.ID
	dst.Name = &plane.Name
	dst.Type = &plane.Type
	dst.Location = &plane.Location
	dst.Tags = *to.StringMapPtr(plane.Tags)
	dst.SystemData = fromSystemDataModel(plane.SystemData)

	dst.Properties = &RadiusPlaneResourceProperties{
		ProvisioningState: fromProvisioningStateDataModel(plane.InternalMetadata.AsyncProvisioningState),
		ResourceProviders: *to.StringMapPtr(plane.Properties.ResourceProviders),
	}

	return nil
}
