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

// ConvertTo converts from the versioned ResourceProviderResource resource to version-agnostic datamodel.
func (src *ResourceProviderResource) ConvertTo() (v1.DataModelInterface, error) {
	dst := &datamodel.ResourceProvider{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     datamodel.ResourceProviderResourceType,
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},
	}

	return dst, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned ResourceProviderResource resource.
func (dst *ResourceProviderResource) ConvertFrom(src v1.DataModelInterface) error {
	dm, ok := src.(*datamodel.ResourceProvider)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(dm.ID)
	dst.Name = to.Ptr(dm.Name)
	dst.Type = to.Ptr(dm.Type)
	dst.Location = to.Ptr(dm.Location)
	dst.Tags = *to.StringMapPtr(dm.Tags)
	dst.SystemData = fromSystemDataModel(dm.SystemData)

	dst.Properties = &ResourceProviderProperties{
		ProvisioningState: to.Ptr(ProvisioningState(dm.InternalMetadata.AsyncProvisioningState)),
	}

	return nil
}
