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

// ConvertTo converts from the versioned Gateway resource to version-agnostic datamodel.
func (src *GatewayResource) ConvertTo() (api.DataModelInterface, error) {
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

	converted := &datamodel.Gateway{
		TrackedResource: basedatamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.GatewayProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Application:       to.String(src.Properties.Application),
			Hostname: datamodel.GatewayPropertiesHostname{
				FullyQualifiedHostname: to.String(src.Properties.Hostname.FullyQualifiedHostname),
				Prefix:                 to.String(src.Properties.Hostname.Prefix),
			},
			Routes: routes,
		},
		InternalMetadata: basedatamodel.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Gateway resource.
func (dst *GatewayResource) ConvertFrom(src api.DataModelInterface) error {
	g, ok := src.(*datamodel.Gateway)
	if !ok {
		return api.ErrInvalidModelConversion
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

	dst.ID = to.StringPtr(g.ID)
	dst.Name = to.StringPtr(g.Name)
	dst.Type = to.StringPtr(g.Type)
	dst.SystemData = fromSystemDataModel(g.SystemData)
	dst.Location = to.StringPtr(g.Location)
	dst.Tags = *to.StringMapPtr(g.Tags)
	dst.Properties = &GatewayProperties{
		ProvisioningState: fromProvisioningStateDataModel(g.Properties.ProvisioningState),
		Application:       to.StringPtr(g.Properties.Application),
		Hostname: &GatewayPropertiesHostname{
			FullyQualifiedHostname: to.StringPtr(g.Properties.Hostname.FullyQualifiedHostname),
			Prefix:                 to.StringPtr(g.Properties.Hostname.Prefix),
		},
		Routes: routes,
	}

	return nil
}
