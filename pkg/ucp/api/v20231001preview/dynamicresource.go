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

package v20231001preview

// DynamicResource is used as the versioned resource model for dynamic resources.
//
// A dynamic resource is implemented internally to UCP, and uses a user-provided
// OpenAPI specification to define the resource schema. Since the resource is internal
// to UCP and dynamically generated, this struct is used to represent all dynamic resources.
type DynamicResource struct {
	ID         *string            `json:"id"`
	Name       *string            `json:"name"`
	Type       *string            `json:"type"`
	Location   *string            `json:"location"`
	Tags       map[string]*string `json:"tags,omitempty"`
	Properties map[string]any     `json:"properties,omitempty"`
	SystemData *SystemData        `json:"systemData,omitempty"`
}
