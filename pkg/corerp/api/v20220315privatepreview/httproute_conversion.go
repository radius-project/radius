// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned HTTPRoute resource to version-agnostic datamodel.
func (src *HTTPRouteResource) ConvertTo() (conv.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	// TODO: Improve the validation.
	var routes []datamodel.RouteDestination
	for _, e := range src.Properties.Routes {
		routes = append(routes, datamodel.RouteDestination{Destination: to.String(e.Destination), Weight: to.Int32(e.Weight)})
	}
	converted := &datamodel.HTTPRoute{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: &datamodel.HTTPRouteProperties{
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: to.String(src.Properties.Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Hostname:          to.String(src.Properties.Hostname),
			Port:              to.Int32(src.Properties.Port),
			Scheme:            to.String(src.Properties.Scheme),
			URL:               to.String(src.Properties.URL),
			Routes:            routes,
			// ContainerPort:     to.Int32(src.Properties.ContainerPort),
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	if src.Properties.ContainerPort != nil {
		converted.Properties.ContainerPort = to.Int32(src.Properties.ContainerPort)
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned HTTPRoute resource.
func (dst *HTTPRouteResource) ConvertFrom(src conv.DataModelInterface) error {
	// TODO: Improve the validation.
	route, ok := src.(*datamodel.HTTPRoute)
	if !ok {
		return conv.ErrInvalidModelConversion
	}
	var routes []*HTTPRoutePropertiesRoutesItem

	for _, e := range route.Properties.Routes {
		routes = append(routes, &HTTPRoutePropertiesRoutesItem{Destination: to.StringPtr(e.Destination), Weight: to.Int32Ptr(e.Weight)})
	}
	dst.ID = to.StringPtr(route.ID)
	dst.Name = to.StringPtr(route.Name)
	dst.Type = to.StringPtr(route.Type)
	dst.SystemData = fromSystemDataModel(route.SystemData)
	dst.Location = to.StringPtr(route.Location)
	dst.Tags = *to.StringMapPtr(route.Tags)
	dst.Properties = &HTTPRouteProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: v1.BuildExternalOutputResources(route.Properties.Status.OutputResources),
			},
		},
		ProvisioningState: fromProvisioningStateDataModel(route.Properties.ProvisioningState),
		Application:       to.StringPtr(route.Properties.Application),
		Hostname:          to.StringPtr(route.Properties.Hostname),
		Port:              to.Int32Ptr((route.Properties.Port)),
		Scheme:            to.StringPtr(route.Properties.Scheme),
		URL:               to.StringPtr(route.Properties.URL),
		Routes:            routes,
		ContainerPort:     to.Int32Ptr(route.Properties.ContainerPort),
	}
	if route.Properties.ContainerPort == 0 {
		dst.Properties.ContainerPort = nil
	}
	return nil
}
