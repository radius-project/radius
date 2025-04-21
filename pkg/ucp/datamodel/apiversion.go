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
	// APIVersionResourceType is the resource type for an API version.
	APIVersionResourceType = "System.Resources/resourceProviders/resourceTypes/apiVersions"
)

// APIVersion represents an API version of a resource type.
type APIVersion struct {
	v1.BaseResource

	// Properties stores the properties of the API version.
	Properties APIVersionProperties `json:"properties"`
}

// ResourceTypeName gives the type of the resource.
func (r *APIVersion) ResourceTypeName() string {
	return APIVersionResourceType
}

// APIVersion stores the properties of an API version.
type APIVersionProperties struct {
	// Empty for now.
	Schema map[string]any
}
