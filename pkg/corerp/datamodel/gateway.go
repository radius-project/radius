// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

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
	return "Applications.Core/gateways"
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (g *Gateway) ApplyDeploymentOutput(do rpv1.DeploymentOutput) {
	g.Properties.Status.OutputResources = do.DeployedOutputResources
	g.ComputedValues = do.ComputedValues
	g.SecretValues = do.SecretValues
	if url, ok := do.ComputedValues["url"].(string); ok {
		g.Properties.URL = url
	}
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
	SSLPassthrough bool `json:"sslPassthrough,omitempty"`
}
