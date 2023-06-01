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

package v20220315privatepreview

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned Gateway resource to version-agnostic datamodel.
func (src *GatewayResource) ConvertTo() (v1.DataModelInterface, error) {

	tls := &datamodel.GatewayPropertiesTLS{}
	if src.Properties.TLS == nil {
		tls = nil
	} else {
		if src.Properties.TLS.SSLPassthrough != nil {
			tls.SSLPassthrough = to.Bool(src.Properties.TLS.SSLPassthrough)
		} else {
			tls.SSLPassthrough = false
		}

		if src.Properties.TLS.CertificateFrom != nil {
			tls.CertificateFrom = to.String(src.Properties.TLS.CertificateFrom)
			tls.Hostname = to.String(src.Properties.TLS.Hostname)
			tls.MinimumProtocolVersion = toTLSMinVersionDataModel(src.Properties.TLS.MinimumProtocolVersion)
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
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
func (dst *GatewayResource) ConvertFrom(src v1.DataModelInterface) error {
	g, ok := src.(*datamodel.Gateway)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	var tls *GatewayPropertiesTLS
	if g.Properties.TLS != nil {
		tls = &GatewayPropertiesTLS{
			CertificateFrom:        to.Ptr(g.Properties.TLS.CertificateFrom),
			Hostname:               to.Ptr(g.Properties.TLS.Hostname),
			MinimumProtocolVersion: fromTLSMinVersionDataModel(g.Properties.TLS.MinimumProtocolVersion),
			SSLPassthrough:         to.Ptr(g.Properties.TLS.SSLPassthrough),
		}
	}

	routes := []*GatewayRoute{}
	if g.Properties.Routes != nil {
		for _, r := range g.Properties.Routes {
			s := &GatewayRoute{
				Destination:   to.Ptr(r.Destination),
				Path:          to.Ptr(r.Path),
				ReplacePrefix: to.Ptr(r.ReplacePrefix),
			}
			routes = append(routes, s)
		}
	}

	var hostname *GatewayPropertiesHostname
	if g.Properties.Hostname != nil {
		hostname = &GatewayPropertiesHostname{
			FullyQualifiedHostname: to.Ptr(g.Properties.Hostname.FullyQualifiedHostname),
			Prefix:                 to.Ptr(g.Properties.Hostname.Prefix),
		}
	}

	dst.ID = to.Ptr(g.ID)
	dst.Name = to.Ptr(g.Name)
	dst.Type = to.Ptr(g.Type)
	dst.SystemData = fromSystemDataModel(g.SystemData)
	dst.Location = to.Ptr(g.Location)
	dst.Tags = *to.StringMapPtr(g.Tags)
	dst.Properties = &GatewayProperties{
		Status: &ResourceStatus{
			OutputResources: rpv1.BuildExternalOutputResources(g.Properties.Status.OutputResources),
		},
		ProvisioningState: fromProvisioningStateDataModel(g.InternalMetadata.AsyncProvisioningState),
		Application:       to.Ptr(g.Properties.Application),
		Hostname:          hostname,
		Routes:            routes,
		TLS:               tls,
		URL:               to.Ptr(g.Properties.URL),
	}

	return nil
}

func toTLSMinVersionDataModel(tlsMinVersion *TLSMinVersion) datamodel.MinimumTLSProtocolVersion {
	switch *tlsMinVersion {
	case TLSMinVersionOne2:
		return datamodel.TLSMinVersion12
	case TLSMinVersionOne3:
		return datamodel.TLSMinVersion13
	default:
		return datamodel.TLSMinVersion12
	}
}

func fromTLSMinVersionDataModel(tlsMinVersion datamodel.MinimumTLSProtocolVersion) *TLSMinVersion {
	var t TLSMinVersion
	switch tlsMinVersion {
	case datamodel.TLSMinVersion12:
		t = TLSMinVersionOne2
	case datamodel.TLSMinVersion13:
		t = TLSMinVersionOne3
	default:
		t = TLSMinVersionOne2
	}

	return &t
}
