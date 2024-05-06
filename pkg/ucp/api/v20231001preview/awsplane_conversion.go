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

// ConvertTo converts from the versioned AWS Plane resource to version-agnostic datamodel.
func (src *AwsPlaneResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.AWSPlane{
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
		Properties: datamodel.AWSPlaneProperties{}, // Empty
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned AWS Plane resource.
func (dst *AwsPlaneResource) ConvertFrom(src v1.DataModelInterface) error {
	plane, ok := src.(*datamodel.AWSPlane)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &plane.ID
	dst.Name = &plane.Name
	dst.Type = &plane.Type
	dst.Location = &plane.Location
	dst.Tags = *to.StringMapPtr(plane.Tags)
	dst.SystemData = fromSystemDataModel(plane.SystemData)

	dst.Properties = &AwsPlaneResourceProperties{
		ProvisioningState: fromProvisioningStateDataModel(plane.InternalMetadata.AsyncProvisioningState),
	}

	return nil
}
