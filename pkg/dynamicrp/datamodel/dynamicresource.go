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

var _ v1.ResourceDataModel = (*DynamicResource)(nil)

// DynamicResource is used as the data model for dynamic resources (UDT).
//
// A dynamic resource uses a user-provided OpenAPI specification to define the resource schema. Therefore,
// the properties of the resource are not known at compile time.
type DynamicResource struct {
	v1.BaseResource

	// Properties stores the properties of the resource being tracked.
	Properties map[string]any `json:"properties"`
}
