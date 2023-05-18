// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

const HTTPRouteResourceType = "Applications.Core/httpRoutes"

// HTTPRoute represents HTTPRoute resource.
type HTTPRoute struct {
	v1.BaseResource

	// TODO: remove this from CoreRP
	LinkMetadata
	// Properties is the properties of the resource.
	Properties *HTTPRouteProperties `json:"properties"`
}

// ResourceTypeName returns the qualified name of the resource
func (h *HTTPRoute) ResourceTypeName() string {
	return HTTPRouteResourceType
}

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (h *HTTPRoute) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	if h.Properties != nil {
		h.Properties.Status.OutputResources = do.DeployedOutputResources
	}

	h.ComputedValues = do.ComputedValues
	h.SecretValues = do.SecretValues

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
	return nil
}

// OutputResources returns the output resources array.
func (h *HTTPRoute) OutputResources() []rpv1.OutputResource {
	return h.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (h *HTTPRoute) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &h.Properties.BasicResourceProperties
}

// HTTPRouteProperties represents the properties of HTTPRoute.
type HTTPRouteProperties struct {
	rpv1.BasicResourceProperties
	Hostname string `json:"hostname,omitempty"`
	Port     int32  `json:"port,omitempty"`
	Scheme   string `json:"scheme,omitempty"`
	URL      string `json:"url,omitempty"`
}
