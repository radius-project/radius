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

package objectformats

import (
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/resources/radius"
)

// ResourceIDToResourceGroupNameTransformer is a transformer that takes a resource ID and returns the resource group name.
type ResourceIDToResourceGroupNameTransformer struct {
}

// Transform takes a resource ID and returns the resource group name.
func (t *ResourceIDToResourceGroupNameTransformer) Transform(input string) string {
	if input == "" {
		return ""
	}

	// NOTE: this is for display to human users in a table. It's not a great place
	// for us to put a long explanation.
	id, err := resources.ParseResource(input)
	if err != nil {
		return "<error>"
	}

	return id.FindScope(radius.ScopeResourceGroups)
}

// ResourceIDToResourceNameTransformer is a transformer that takes a resource ID and returns the resource name.
type ResourceIDToResourceNameTransformer struct {
}

// Transform takes a resource ID and returns the resource name.
func (t *ResourceIDToResourceNameTransformer) Transform(input string) string {
	if input == "" {
		return ""
	}

	// NOTE: this is for display to human users in a table. It's not a great place
	// for us to put a long explanation.
	id, err := resources.ParseResource(input)
	if err != nil {
		return "<error>"
	}

	return id.Name()
}
