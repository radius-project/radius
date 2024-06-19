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

import v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"

const (
	// ResourceType is the resource type for a resource provider.
	ResourceProviderResourceType = "System.Resources/resourceProviders"
)

// ResourceProvider represents a resource provider (namespace + set of types).
type ResourceProvider struct {
	v1.BaseResource

	// Properties stores the properties of the resource provider.
	Properties ResourceProviderProperties `json:"properties"`
}

// ResourceTypeName gives the type of the resource.
func (r *ResourceProvider) ResourceTypeName() string {
	return ResourceProviderResourceType
}

// ReosurceProviderID returns the resource ID of the resource provider.
func ResourceProviderID(scope string, namespace string) string {
	return scope + "/providers/" + (&ResourceProvider{}).ResourceTypeName() + "/" + namespace
}

// ResourceProviderProperties stores the properties of a resource provider.
type ResourceProviderProperties struct {
	// Locations is the list of locations supported by this resource provider.
	Locations map[string]ResourceProviderLocation `json:"locations"`

	// ResourceTypes stores the properties of the resource types.
	ResourceTypes []ResourceType `json:"resourceTypes"`
}

// ResourceProviderLocation stores the configuration for each instance of the resource provider.
type ResourceProviderLocation struct {
	// Address is the address of the resource provider for this location.
	Address string `json:"address"`
}

type ResourceTypeAPIVersion struct {
	// Schema is the OpenAPI v3 schema of the resource type.
	Schema map[string]any `json:"schema"`
}

// ResourceType stores the properties of a resource type.
type ResourceType struct {
	// ResourceType is the name of the resource type.
	ResourceType string `json:"resourceType"`

	// APIVersions is the list of API versions supported by this resource type.
	APIVersions map[string]ResourceTypeAPIVersion `json:"apiVersions"`

	// Capabilities is the list of capabilities of this resource type.
	Capabilities []string `json:"capabilities"`

	// DefaultAPIVersion is the default API version for this resource type.
	DefaultAPIVersion string `json:"defaultApiVersion"`

	// Locations is the list of locations supported by this resource type.
	Locations []string `json:"locations"`
}
