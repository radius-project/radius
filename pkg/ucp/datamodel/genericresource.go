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

// GenericResource represents a stored "tracked resource" within a UCP resource group.
//
// This type is used to store tracked resources within UCP regardless of the actual
// resource type. You can think of it as a "meta-resource". The top level fields like "ID",
// "Name", and "Type" reflect the GenericResource entry itself. The actual resource data
// is stored in the "Properties" field.
//
// GenericResource are returned through the resource list APIs, but don't support PUT or
// DELETE operations directly. The resource ID, Name, and Type of the GenericResource
// are an implementation detail and are never exposed to users.
type GenericResource struct {
	v1.BaseResource

	// Properties stores the properties of the resource being tracked.
	Properties GenericResourceProperties `json:"properties"`
}

// ResourceTypeName gives the type of ucp resource.
func (r *GenericResource) ResourceTypeName() string {
	return "System.Resources/resources"
}

// GenericResourceProperties stores the properties of the resource being tracked.
//
// Right now we only track the basic identifiers. This is enough for UCP to remebmer
// which resources exist, but not to act as a cache. We may want to add more fields
// in the future as we support additional scenarios.
type GenericResourceProperties struct {
	// ID is the fully qualified resource ID for the resource.
	ID string `json:"id"`
	// Name is the resource name.
	Name string `json:"name"`
	// Type is the resource type.
	Type string `json:"type"`
}
