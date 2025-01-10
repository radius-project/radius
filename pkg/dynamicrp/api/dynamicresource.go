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

package api

// DynamicResource is used as the versioned resource model for dynamic resources.
//
// A dynamic resource uses a user-provided OpenAPI specification to define the resource schema. Therefore,
// the properties of the resource are not known at compile time.
type DynamicResource struct {
	// ID is the resource ID.
	ID *string `json:"id"`
	// Name is the resource name.
	Name *string `json:"name"`
	// Type is the resource type.
	Type *string `json:"type"`
	// Location is the resource location.
	Location *string `json:"location"`
	// Tags are the resource tags.
	Tags map[string]*string `json:"tags,omitempty"`
	// Properties stores the properties of the resource.
	Properties map[string]any `json:"properties,omitempty"`
	// SystemData stores the system data of the resource.
	SystemData map[string]any `json:"systemData,omitempty"`
}
