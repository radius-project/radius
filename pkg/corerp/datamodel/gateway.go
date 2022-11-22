// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
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
func (g *Gateway) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	g.Properties.Status.OutputResources = do.DeployedOutputResources
	g.ComputedValues = do.ComputedValues
	g.SecretValues = do.SecretValues
	if url, ok := do.ComputedValues["url"].(string); ok {
		g.Properties.URL = url
	}
}

// OutputResources returns the output resources array.
func (g *Gateway) OutputResources() []outputresource.OutputResource {
	return g.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (h *Gateway) ResourceMetadata() *rp.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// GatewayProperties represents the properties of Gateway.
type GatewayProperties struct {
	rp.BasicResourceProperties
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
	SSLPassThrough bool `json:"sslPassThrough,omitempty"`
}
