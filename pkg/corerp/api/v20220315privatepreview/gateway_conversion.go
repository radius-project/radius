// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Gateway resource to version-agnostic datamodel.
func (src *GatewayResource) ConvertTo() (conv.DataModelInterface, error) {

	tls := &datamodel.GatewayPropertiesTLS{}
	if src.Properties.TLS == nil {
		tls = nil
	} else {
		if src.Properties.TLS.SSLPassThrough != nil {
			tls.SSLPassThrough = to.Bool(src.Properties.TLS.SSLPassThrough)
		} else {
			tls.SSLPassThrough = false
		}
	}

	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	routes := []datamodel.GatewayRoute{}
	if src.Properties.Routes != nil {
		for _, r := range src.Properties.Routes {
			s := datamodel.GatewayRoute{
				Destination:   to.String(r.Destination),
				Path:          to.String(r.Path),
				ReplacePrefix: to.String(r.ReplacePrefix),
			}
			routes = append(routes, s)
		}
	}

	var hostname *datamodel.GatewayPropertiesHostname
	if src.Properties.Hostname != nil {
		hostname = &datamodel.GatewayPropertiesHostname{
			FullyQualifiedHostname: to.String(src.Properties.Hostname.FullyQualifiedHostname),
			Prefix:                 to.String(src.Properties.Hostname.Prefix),
		}
	}

	converted := &datamodel.Gateway{
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
		Properties: datamodel.GatewayProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: to.String(src.Properties.Application),
			},
			Hostname: hostname,
			TLS:      tls,
			Routes:   routes,
			URL:      to.String(src.Properties.URL),
		},
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Gateway resource.
func (dst *GatewayResource) ConvertFrom(src conv.DataModelInterface) error {
	g, ok := src.(*datamodel.Gateway)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	var tls *GatewayPropertiesTLS
	if g.Properties.TLS != nil {
		tls = &GatewayPropertiesTLS{
			SSLPassThrough: to.BoolPtr(g.Properties.TLS.SSLPassThrough),
		}
	}

	routes := []*GatewayRoute{}
	if g.Properties.Routes != nil {
		for _, r := range g.Properties.Routes {
			s := &GatewayRoute{
				Destination:   to.StringPtr(r.Destination),
				Path:          to.StringPtr(r.Path),
				ReplacePrefix: to.StringPtr(r.ReplacePrefix),
			}
			routes = append(routes, s)
		}
	}

	var hostname *GatewayPropertiesHostname
	if g.Properties.Hostname != nil {
		hostname = &GatewayPropertiesHostname{
			FullyQualifiedHostname: to.StringPtr(g.Properties.Hostname.FullyQualifiedHostname),
			Prefix:                 to.StringPtr(g.Properties.Hostname.Prefix),
		}
	}

	dst.ID = to.StringPtr(g.ID)
	dst.Name = to.StringPtr(g.Name)
	dst.Type = to.StringPtr(g.Type)
	dst.SystemData = fromSystemDataModel(g.SystemData)
	dst.Location = to.StringPtr(g.Location)
	dst.Tags = *to.StringMapPtr(g.Tags)
	dst.Properties = &GatewayProperties{
		Status: &ResourceStatus{
			OutputResources: rp.BuildExternalOutputResources(g.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(g.InternalMetadata.AsyncProvisioningState),
		Application:       to.StringPtr(g.Properties.Application),
		Hostname:          hostname,
		Routes:            routes,
		TLS:               tls,
		URL:               to.StringPtr(g.Properties.URL),
	}

	return nil
}
