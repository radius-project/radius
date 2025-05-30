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
	"strings"

	"github.com/radius-project/radius/pkg/cli/output"
)

// GetResourceTableFormat returns the fields to output from a resource object.
// This function should be used with the generated CoreRP and other portable resource types.
func GetResourceTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RESOURCE",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .Type }",
			},
			{
				Heading:     "GROUP",
				JSONPath:    "{ .ID }",
				Transformer: &ResourceIDToResourceGroupNameTransformer{},
			},
			{
				Heading:  "STATE",
				JSONPath: "{ .Properties.ProvisioningState }",
			},
		},
	}
}

// GetGenericResourceTableFormat returns the fields to output from a generic resource object.
// This function should be used with the Go type GenericResource.
// The difference between this function and the GetResourceTableFormat function above is that
// GenericResource properties is a map and the way to get the ProvisioningState is different.
func GetGenericResourceTableFormat() output.FormatterOptions {
	return output.FormatterOptions{
		Columns: []output.Column{
			{
				Heading:  "RESOURCE",
				JSONPath: "{ .Name }",
			},
			{
				Heading:  "TYPE",
				JSONPath: "{ .Type }",
			},
			{
				Heading:     "GROUP",
				JSONPath:    "{ .ID }",
				Transformer: &ResourceIDToResourceGroupNameTransformer{},
			},
			{
				Heading:     "ENVIRONMENT",
				JSONPath:    "{ .Properties.environment }",
				Transformer: &ResourceEnvironmentNameTransformer{},
			},
			{
				Heading:  "STATE",
				JSONPath: "{ .Properties.provisioningState }",
			},
		},
	}
}

// ResourceEnvironmentNameTransformer extracts the environment name from a fully qualified environment path.
type ResourceEnvironmentNameTransformer struct {
}

// Transform extracts the environment name from a fully qualified environment path.
// The path is expected to be in the format '/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env'
// and this function will return 'test-env'.
func (t *ResourceEnvironmentNameTransformer) Transform(value string) string {
	if value == "" {
		return "default"
	}

	// Extract just the environment name from the fully qualified path
	parts := strings.Split(value, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return "default"
}

// ResourceIDToResourceGroupNameTransformer extracts the resource group name from a resource ID.
