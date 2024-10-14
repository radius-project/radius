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
	// ResourceTypesResourceType is the resource type for a resource type.
	ResourceTypeResourceType = "System.Resources/resourceProviders/resourceTypes"
)

// ResourceType represents a resource type.
type ResourceType struct {
	v1.BaseResource

	// Properties stores the properties of the resource type.
	Properties ResourceTypeProperties `json:"properties"`
}

// ResourceTypeName gives the type of the resource.
func (r *ResourceType) ResourceTypeName() string {
	return ResourceTypeResourceType
}

// ResourceTypeProperties stores the properties of a resource type.
type ResourceTypeProperties struct {
	// DefaultAPIVersion is the default API version for this resource type.
	DefaultAPIVersion *string `json:"defaultApiVersion"`
}
