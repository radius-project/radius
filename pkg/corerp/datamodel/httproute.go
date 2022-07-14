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

// HTTPRoute represents HTTPRoute resource.
type HTTPRoute struct {
	v1.TrackedResource

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties *HTTPRouteProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (h *HTTPRoute) ResourceTypeName() string {
	return "Applications.Core/httpRoutes"
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (h *HTTPRoute) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	h.Properties.Status.OutputResources = do.DeployedOutputResources
	h.InternalMetadata.ComputedValues = do.ComputedValues
	h.InternalMetadata.SecretValues = do.SecretValues

	if port, ok := do.ComputedValues["port"].(int32); ok {
		h.Properties.Port = port
	}
	if hostname, ok := do.ComputedValues["hostname"].(string); ok {
		h.Properties.Hostname = hostname
	}
	if scheme, ok := do.ComputedValues["scheme"].(string); ok {
		h.Properties.Scheme = scheme
	}
	if url, ok := do.ComputedValues["url"].(string); ok {
		h.Properties.URL = url
	}
}

// OutputResources returns the output resources array.
func (h *HTTPRoute) OutputResources() []outputresource.OutputResource {
	return h.Properties.Status.OutputResources
}

// HTTPRouteProperties represents the properties of HTTPRoute.
type HTTPRouteProperties struct {
	v1.BasicResourceProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Application       string               `json:"application,omitempty"`
	Hostname          string               `json:"hostname,omitempty"`
	Port              int32                `json:"port,omitempty"`
	Scheme            string               `json:"scheme,omitempty"`
	URL               string               `json:"url,omitempty"`
}
