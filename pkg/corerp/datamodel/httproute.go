// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
)

// HTTPRoute represents HTTPRoute resource.
type HTTPRoute struct {
	TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData armrpcv1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties HTTPRouteProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	InternalMetadata InternalMetadata `json:"internalMetadata"`
}

// ResourceTypeName returns the qualified name of the resource
func (h HTTPRoute) ResourceTypeName() string {
	return "Applications.Core/httproutes"
}

// HTTPRouteProperties represents the properties of HTTPRoute.
type HTTPRouteProperties struct {
	BasicRouteProperties
	ProvisioningState ProvisioningStates `json:"provisioningState,omitempty"`
	Application       string             `json:"application,omitempty"`
	Hostname          string             `json:"hostname,omitempty"`
	Port              int32              `json:"port,omitempty"`
	Scheme            string             `json:"scheme,omitempty"`
	URL               string             `json:"url,omitempty"`
}

// BasicRouteProperties - Basic properties of a route.
type BasicRouteProperties struct {
	// Status of the resource
	Status RouteStatus `json:"status,omitempty"`
}

// RouteStatus - Status of a route.
type RouteStatus struct {
	// Health state of the route
	HealthState     string                   `json:"healthState,omitempty"`
	OutputResources []map[string]interface{} `json:"outputResources,omitempty"`

	// Provisioning state of the route
	ProvisioningState string `json:"provisioningState,omitempty"`
}
