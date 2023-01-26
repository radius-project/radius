// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned HTTPRoute resource to version-agnostic datamodel.
func (src *HTTPRouteResource) ConvertTo() (v1.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	// TODO: Improve the validation.
	converted := &datamodel.HTTPRoute{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: &datamodel.HTTPRouteProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: to.String(src.Properties.Application),
			},
			Hostname: to.String(src.Properties.Hostname),
			Port:     to.Int32(src.Properties.Port),
			Scheme:   to.String(src.Properties.Scheme),
			URL:      to.String(src.Properties.URL),
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned HTTPRoute resource.
func (dst *HTTPRouteResource) ConvertFrom(src v1.DataModelInterface) error {
	// TODO: Improve the validation.
	route, ok := src.(*datamodel.HTTPRoute)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(route.ID)
	dst.Name = to.StringPtr(route.Name)
	dst.Type = to.StringPtr(route.Type)
	dst.SystemData = fromSystemDataModel(route.SystemData)
	dst.Location = to.StringPtr(route.Location)
	dst.Tags = *to.StringMapPtr(route.Tags)
	dst.Properties = &HTTPRouteProperties{
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(route.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(route.InternalMetadata.AsyncProvisioningState),
		Application:       to.StringPtr(route.Properties.Application),
		Hostname:          to.StringPtr(route.Properties.Hostname),
		Port:              to.Int32Ptr((route.Properties.Port)),
		Scheme:            to.StringPtr(route.Properties.Scheme),
		URL:               to.StringPtr(route.Properties.URL),
	}

	return nil
}
