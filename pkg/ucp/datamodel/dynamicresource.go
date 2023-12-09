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

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

// DynamicResource is used as the data model for dynamic resources.
//
// A dynamic resource is implemented internally to UCP, and uses a user-provided
// OpenAPI specification to define the resource schema. Since the resource is internal
// to UCP and dynamically generated, this struct is used to represent all dynamic resources.
type DynamicResource struct {
	v1.BaseResource

	// Properties stores the properties of the resource being tracked.
	Properties map[string]any `json:"properties"`
}

// ResourceTypeName gives the type of the resource.
func (r *DynamicResource) ResourceTypeName() string {
	return r.Type
}
