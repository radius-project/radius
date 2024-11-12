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

package manifest

// ResourceProvider represents a resource provider manifest.
type ResourceProvider struct {
	// Name is the resource provider name. This is also the namespace of the types defined by the resource provider.
	Name string `yaml:"name" validate:"required,resourceProviderNamespace"`

	// Types is a map of resource types in the resource provider.
	Types map[string]*ResourceType `yaml:"types" validate:"dive,keys,resourceType,endkeys,required"`
}

// ResourceType represents a resource type in a resource provider manifest.
type ResourceType struct {
	// DefaultAPIVersion is the default API version for the resource type.
	DefaultAPIVersion *string `yaml:"defaultApiVersion,omitempty" validate:"omitempty,apiVersion"`

	// APIVersions is a map of API versions for the resource type.
	APIVersions map[string]*ResourceTypeAPIVersion `yaml:"apiVersions" validate:"dive,keys,apiVersion,endkeys,required"`
}

type ResourceTypeAPIVersion struct {
	// Schema is the schema for the resource type.
	//
	// TODO: this allows anything right now, and will be ignored. We'll improve this in
	// a future pull-request.
	Schema any `yaml:"schema" validate:"required"`

	// Capabilities is a list of capabilities for the resource type.
	Capabilities []string `yaml:"capabilities" validate:"dive,capability"`
}
