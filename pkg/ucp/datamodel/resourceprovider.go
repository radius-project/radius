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
	// ResourceProviderResourceType is the resource type for a resource provider.
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

// ResourceProviderProperties stores the properties of a resource provider.
type ResourceProviderProperties struct {
	// Empty for now.
}
