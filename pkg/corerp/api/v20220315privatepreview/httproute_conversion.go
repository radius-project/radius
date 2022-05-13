// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/corerp/api"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned HTTPRoute resource to version-agnostic datamodel.
func (src *HTTPRouteResource) ConvertTo() (api.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	// TODO: Improve the validation.
	converted := &datamodel.HTTPRoute{
		TrackedResource: datamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.HTTPRouteProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Application:       to.String(src.Properties.Application),
			Hostname:          to.String(src.Properties.Hostname),
			Port:              to.Int32(src.Properties.Port),
			Scheme:            to.String(src.Properties.Scheme),
			URL:               to.String(src.Properties.URL),
		},
		InternalMetadata: datamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned HTTPRoute resource.
func (dst *HTTPRouteResource) ConvertFrom(src api.DataModelInterface) error {
	// TODO: Improve the validation.
	route, ok := src.(*datamodel.HTTPRoute)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(route.ID)
	dst.Name = to.StringPtr(route.Name)
	dst.Type = to.StringPtr(route.Type)
	dst.SystemData = fromSystemDataModel(route.SystemData)
	dst.Location = to.StringPtr(route.Location)
	dst.Tags = *to.StringMapPtr(route.Tags)
	dst.Properties = &HTTPRouteProperties{
		ProvisioningState: fromProvisioningStateDataModel(route.Properties.ProvisioningState),
		Application:       to.StringPtr(route.Properties.Application),
		Hostname:          to.StringPtr(route.Properties.Hostname),
		Port:              to.Int32Ptr((route.Properties.Port)),
		Scheme:            to.StringPtr(route.Properties.Scheme),
		URL:               to.StringPtr(route.Properties.URL),
	}

	return nil
}
