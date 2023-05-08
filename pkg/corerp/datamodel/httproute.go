/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

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
