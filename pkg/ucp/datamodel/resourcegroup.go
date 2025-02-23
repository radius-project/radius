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
	// ResourceGroupResourceType is the type of a resource group.
	ResourceGroupResourceType = "System.Resources/resourceGroups"
)

// ResourceGroup represents UCP ResourceGroup.
type ResourceGroup struct {
	v1.BaseResource
}

// ResourceTypeName returns a string representing the resource type name of the ResourceGroup object.
func (p ResourceGroup) ResourceTypeName() string {
	return ResourceGroupResourceType
}
