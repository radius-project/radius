// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/rp"
)

// Gateway represents Gateway resource.
type Gateway struct {
	v1.TrackedResource

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
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
	g.InternalMetadata.ComputedValues = do.ComputedValues
	g.InternalMetadata.SecretValues = do.SecretValues
	// TODO gateway should have a url output property.
}

// OutputResources returns the output resources array.
func (g *Gateway) OutputResources() []outputresource.OutputResource {
	return g.Properties.Status.OutputResources
}

// GatewayProperties represents the properties of Gateway.
type GatewayProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState       `json:"provisioningState,omitempty"`
	Application       string                     `json:"application,omitempty"`
	Internal          bool                       `json:"internal,omitempty"`
	Hostname          *GatewayPropertiesHostname `json:"hostname,omitempty"`
	Routes            []GatewayRoute             `json:"routes,omitempty"`
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
