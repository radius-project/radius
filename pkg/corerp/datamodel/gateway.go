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

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

const GatewayResourceType = "Applications.Core/gateways"

// Gateway represents Gateway resource.
type Gateway struct {
	v1.BaseResource

	// TODO: remove this from CoreRP
	LinkMetadata
	// Properties is the properties of the resource.
	Properties GatewayProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (g *Gateway) ResourceTypeName() string {
	return GatewayResourceType
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (g *Gateway) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	g.Properties.Status.OutputResources = do.DeployedOutputResources
	g.ComputedValues = do.ComputedValues
	g.SecretValues = do.SecretValues
	if url, ok := do.ComputedValues["url"].(string); ok {
		g.Properties.URL = url
	}
	return nil
}

// OutputResources returns the output resources array.
func (g *Gateway) OutputResources() []rpv1.OutputResource {
	return g.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (h *Gateway) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// GatewayProperties represents the properties of Gateway.
type GatewayProperties struct {
	rpv1.BasicResourceProperties
	Internal bool                       `json:"internal,omitempty"`
	Hostname *GatewayPropertiesHostname `json:"hostname,omitempty"`
	TLS      *GatewayPropertiesTLS      `json:"tls,omitempty"`
	Routes   []GatewayRoute             `json:"routes,omitempty"`
	URL      string                     `json:"url,omitempty"`
}

// GatewayRoute represents the route attached to Gateway.
type GatewayRoute struct {
	Destination   string `json:"destination,omitempty"`
	Path          string `json:"path,omitempty"`
	ReplacePrefix string `json:"replacePrefix,omitempty"`
}

// GatewayPropertiesHostname - Declare hostname information for the Gateway.
type GatewayPropertiesHostname struct {
	FullyQualifiedHostname string `json:"fullyQualifiedHostname,omitempty"`
	Prefix                 string `json:"prefix,omitempty"`
}

// GatewayPropertiesTLS - Declare TLS information for the Gateway.
type GatewayPropertiesTLS struct {
	SSLPassthrough         bool                      `json:"sslPassthrough,omitempty"`
	Hostname               string                    `json:"hostname,omitempty"`
	MinimumProtocolVersion MinimumTLSProtocolVersion `json:"minimumProtocolVersion,omitempty"`
	CertificateFrom        string                    `json:"certificateFrom,omitempty"`
}

// IsValid returns whether or not the supplied MinimumTLSProtocolVersion is supported.
func (m MinimumTLSProtocolVersion) IsValid() bool {
	s := ValidMinimumTLSProtocolVersions()
	for _, v := range s {
		if v == m {
			return true
		}
	}
	return false
}

// IsEqualTo returns whether or not two MinimumTLSProtocolVersions are equal.
func (m MinimumTLSProtocolVersion) IsEqualTo(minumumTLSProtocolVersion MinimumTLSProtocolVersion) bool {
	return m == minumumTLSProtocolVersion
}

// MinimumTLSProtocolVersion represents the minimum TLS protocol version supported by the Gateway.
type MinimumTLSProtocolVersion string

const (
	// TLS 1.2
	TLSMinVersion12 MinimumTLSProtocolVersion = "1.2"
	// TLS 1.3
	TLSMinVersion13 MinimumTLSProtocolVersion = "1.3"
)

// ValidMinimumTLSProtocolVersions returns a list of valid MinimumTLSProtocolVersions.
func ValidMinimumTLSProtocolVersions() []MinimumTLSProtocolVersion {
	return []MinimumTLSProtocolVersion{
		TLSMinVersion12,
		TLSMinVersion13,
	}
}
