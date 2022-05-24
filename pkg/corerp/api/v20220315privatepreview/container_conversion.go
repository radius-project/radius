// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Container resource to version-agnostic datamodel.
func (src *ContainerResource) ConvertTo() (api.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	converted := &datamodel.Container{
		TrackedResource: basedatamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.ContainerProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Application:       to.String(src.Properties.Application),
		},
		InternalMetadata: basedatamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Container resource.
func (dst *ContainerResource) ConvertFrom(src api.DataModelInterface) error {
	route, ok := src.(*datamodel.Container)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(route.ID)
	dst.Name = to.StringPtr(route.Name)
	dst.Type = to.StringPtr(route.Type)
	dst.SystemData = fromSystemDataModel(route.SystemData)
	dst.Location = to.StringPtr(route.Location)
	dst.Tags = *to.StringMapPtr(route.Tags)
	dst.Properties = &ContainerProperties{
		ProvisioningState: fromProvisioningStateDataModel(route.Properties.ProvisioningState),
		Application:       to.StringPtr(route.Properties.Application),
	}

	return nil
}
