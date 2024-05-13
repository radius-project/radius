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
	"errors"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"

	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// ConvertTo converts from the versioned generic Plane resource to version-agnostic datamodel.
func (src *GenericPlaneResource) ConvertTo() (v1.DataModelInterface, error) {
	return nil, errors.New("not implemented")
}

// ConvertFrom converts from version-agnostic datamodel to the versioned generic Plane resource.
func (dst *GenericPlaneResource) ConvertFrom(src v1.DataModelInterface) error {
	plane, ok := src.(*datamodel.GenericPlane)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = &plane.ID
	dst.Name = &plane.Name
	dst.Type = &plane.Type
	dst.Location = &plane.Location
	dst.Tags = *to.StringMapPtr(plane.Tags)
	dst.SystemData = fromSystemDataModel(plane.SystemData)

	// Right now we don't output any of the properties of a plane. We can't know if they contain secrets.
	dst.Properties = &GenericPlaneResourceProperties{
		ProvisioningState: fromProvisioningStateDataModel(plane.InternalMetadata.AsyncProvisioningState),
	}

	return nil
}
