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
	// LocationResourceType is the resource type for a resource provider location
	LocationResourceType = "System.Resources/resourceProviders/locations"

	// LocationUnqualifiedResourceType is the unqualified resource type for a resource provider location.
	LocationUnqualifiedResourceType = "locations"
)

// Location represents a location.
type Location struct {
	v1.BaseResource

	// Properties stores the properties of the location.
	Properties LocationProperties `json:"properties"`
}

// ResourceTypeName gives the type of the resource.
func (r *Location) ResourceTypeName() string {
	return LocationResourceType
}

// LocationProperties stores the properties of the location
type LocationProperties struct {
	// Address is the address (url) of the resource provider.
	Address *string `json:"address,omitempty"`

	// ResourceTypes defines the configuration for resource types supported in this location.
	ResourceTypes map[string]LocationResourceTypeConfiguration `json:"resourceTypes,omitempty"`
}

// LocationResourceTypeConfiguration represents the configuration for resource type in a location.
type LocationResourceTypeConfiguration struct {
	// APIVersions defines the configuration for API versions supported for this resource type in this location.
	APIVersions map[string]LocationAPIVersionConfiguration `json:"apiVersions,omitempty"`
}

// LocationAPIVersionConfiguration represents the configuration for an API version in a location.
type LocationAPIVersionConfiguration struct {
	// Empty for now.
}
