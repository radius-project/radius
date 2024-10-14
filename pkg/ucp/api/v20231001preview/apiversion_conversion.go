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

// ConvertTo converts from the versioned APIVersionResource resource to version-agnostic datamodel.
func (src *APIVersionResource) ConvertTo() (v1.DataModelInterface, error) {
	dst := &datamodel.APIVersion{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   to.String(src.ID),
				Name: to.String(src.Name),
				Type: datamodel.APIVersionResourceType,

				// NOTE: this is a child resource. It does not have a location, systemData, or tags.
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},
	}

	dst.Properties = datamodel.APIVersionProperties{}

	return dst, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned APIVersionResource resource.
func (dst *APIVersionResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.APIVersion)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(dm.ID)
	dst.Name = to.Ptr(dm.Name)
	dst.Type = to.Ptr(dm.Type)

	// NOTE: this is a child resource. It does not have a location, systemData, or tags.

	dst.Properties = &APIVersionProperties{
		ProvisioningState: to.Ptr(ProvisioningState(dm.InternalMetadata.AsyncProvisioningState)),
	}

	return nil
}
