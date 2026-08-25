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

package bicep

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_InspectTemplateResources(t *testing.T) {
	tests := []struct {
		name                        string
		template                    map[string]any
		expectedContainsEnvResource bool
	}{
		{
			name:                        "Nil template",
			template:                    nil,
			expectedContainsEnvResource: false,
		},
		{
			name:                        "Empty template",
			template:                    map[string]any{},
			expectedContainsEnvResource: false,
		},
		{
			name: "Template with missing resources field",
			template: map[string]any{
				"parameters": map[string]any{},
			},
			expectedContainsEnvResource: false,
		},
		{
			name: "Template with empty resources map",
			template: map[string]any{
				"resources": map[string]any{},
			},
			expectedContainsEnvResource: false,
		},
		{
			name: "Template with legacy environment resource",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"type": "Applications.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expectedContainsEnvResource: true,
		},
		{
			name: "Template with Radius.Core environment resource (not deprecated)",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"type": "Radius.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expectedContainsEnvResource: true,
		},
		{
			name: "Template with multiple Applications.Core resources and Radius.Core environment",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"type": "Applications.Core/applications@2023-10-01-preview",
						"name": "my-app",
					},
					"env": map[string]any{
						"type": "Radius.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
					"container": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "my-container",
					},
				},
			},
			expectedContainsEnvResource: true,
		},
		{
			name: "Template with invalid resources format (array instead of map)",
			template: map[string]any{
				"resources": []any{
					map[string]any{
						"type": "Applications.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expectedContainsEnvResource: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InspectTemplateResources(tt.template)
			require.Equal(t, tt.expectedContainsEnvResource, result.ContainsEnvironmentResource)
		})
	}
}

func Test_ContainsEnvironmentResource(t *testing.T) {
	tests := []struct {
		name     string
		template map[string]any
		expected bool
	}{
		{
			name:     "Nil template",
			template: nil,
			expected: false,
		},
		{
			name:     "Empty template",
			template: map[string]any{},
			expected: false,
		},
		{
			name: "Template with missing resources field",
			template: map[string]any{
				"parameters": map[string]any{},
			},
			expected: false,
		},
		{
			name: "Template with empty resources map",
			template: map[string]any{
				"resources": map[string]any{},
			},
			expected: false,
		},
		{
			name: "Template with legacy environment resource",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"type": "Applications.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: true,
		},
		{
			name: "Template with legacy environment resource - case insensitive",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"type": "applications.core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: true,
		},
		{
			name: "Template with multiple resources including environment",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"type": "Applications.Core/applications@2023-10-01-preview",
						"name": "my-app",
					},
					"env": map[string]any{
						"type": "Applications.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
					"container": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "my-container",
					},
				},
			},
			expected: true,
		},
		{
			name: "Template without environment resource",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"type": "Applications.Core/applications@2023-10-01-preview",
						"name": "my-app",
					},
					"container": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "my-container",
					},
				},
			},
			expected: false,
		},
		{
			name: "Template with invalid resources format (array instead of map)",
			template: map[string]any{
				"resources": []any{
					map[string]any{
						"type": "Applications.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: false,
		},
		{
			name: "Template with invalid resource format (not a map)",
			template: map[string]any{
				"resources": map[string]any{
					"env": "not a map",
				},
			},
			expected: false,
		},
		{
			name: "Template with resource missing type field",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"name": "my-env",
					},
				},
			},
			expected: false,
		},
		{
			name: "Template with resource type not a string",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"type": 123,
						"name": "my-env",
					},
				},
			},
			expected: false,
		},
		{
			name: "Template with Radius.Core environment resource type",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"type": "Radius.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: true,
		},
		{
			name: "Template with Radius.Core environment resource type - case insensitive",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"type": "radius.core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: true,
		},
		{
			name: "Template with mixed resource types including Radius.Core environment",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"type": "Applications.Core/applications@2023-10-01-preview",
						"name": "my-app",
					},
					"env": map[string]any{
						"type": "Radius.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsEnvironmentResource(tt.template)
			require.Equal(t, tt.expected, result)
		})
	}
}

func Test_ExtractResourceTypes(t *testing.T) {
	tests := []struct {
		name     string
		template map[string]any
		expected []ResourceTypeEntry
	}{
		{
			name:     "Nil template",
			template: nil,
			expected: nil,
		},
		{
			name:     "Empty template",
			template: map[string]any{},
			expected: nil,
		},
		{
			name: "Template with no resources field",
			template: map[string]any{
				"parameters": map[string]any{},
			},
			expected: nil,
		},
		{
			name: "Template with single resource",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"type": "Radius.Core/applications@2025-08-01-preview",
						"name": "my-app",
					},
				},
			},
			expected: []ResourceTypeEntry{
				{
					FullType:   "Radius.Core/applications@2025-08-01-preview",
					Type:       "Radius.Core/applications",
					APIVersion: "2025-08-01-preview",
				},
			},
		},
		{
			name: "Template with resource missing API version",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"type": "Radius.Core/applications",
						"name": "my-app",
					},
				},
			},
			expected: []ResourceTypeEntry{
				{
					FullType:   "Radius.Core/applications",
					Type:       "Radius.Core/applications",
					APIVersion: "",
				},
			},
		},
		{
			name: "Template with invalid resource format",
			template: map[string]any{
				"resources": map[string]any{
					"app": "not a map",
				},
			},
			expected: nil,
		},
		{
			name: "Template with resource missing type",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"name": "my-app",
					},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractResourceTypes(tt.template)
			require.Equal(t, tt.expected, result)
		})
	}
}

func Test_IsRadiusResourceType(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		expected     bool
	}{
		{
			name:         "Applications.Core type",
			resourceType: "Applications.Core/applications",
			expected:     true,
		},
		{
			name:         "Radius.Core type",
			resourceType: "Radius.Core/environments",
			expected:     true,
		},
		{
			name:         "Applications.Dapr type",
			resourceType: "Applications.Dapr/pubSubBrokers",
			expected:     true,
		},
		{
			name:         "Applications wildcard namespace",
			resourceType: "Applications.Networking/gateways",
			expected:     true,
		},
		{
			name:         "Applications.Datastores type",
			resourceType: "Applications.Datastores/redisCaches",
			expected:     true,
		},
		{
			name:         "Applications.Messaging type",
			resourceType: "Applications.Messaging/rabbitMQQueues",
			expected:     true,
		},
		{
			name:         "Case insensitive match",
			resourceType: "applications.core/containers",
			expected:     true,
		},
		{
			name:         "Azure type",
			resourceType: "Microsoft.Storage/storageAccounts",
			expected:     false,
		},
		{
			name:         "AWS type",
			resourceType: "AWS.S3/Bucket",
			expected:     false,
		},
		{
			name:         "Empty string",
			resourceType: "",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRadiusResourceType(tt.resourceType)
			require.Equal(t, tt.expected, result)
		})
	}
}

func Test_HasOnlyRadiusResourceTypes(t *testing.T) {
	tests := []struct {
		name     string
		template map[string]any
		expected bool
	}{
		{
			name:     "Nil template",
			template: nil,
			expected: false,
		},
		{
			name: "Empty resources",
			template: map[string]any{
				"resources": map[string]any{},
			},
			expected: false,
		},
		{
			name: "Only Radius types",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"type": "Radius.Core/applications@2025-08-01-preview",
					},
					"env": map[string]any{
						"type": "Applications.Core/environments@2023-10-01-preview",
					},
				},
			},
			expected: true,
		},
		{
			name: "Mixed Radius and Azure types",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"type": "Applications.Core/applications@2023-10-01-preview",
					},
					"storage": map[string]any{
						"type": "Microsoft.Storage/storageAccounts@2021-01-01",
					},
				},
			},
			expected: false,
		},
		{
			name: "Only Azure types",
			template: map[string]any{
				"resources": map[string]any{
					"storage": map[string]any{
						"type": "Microsoft.Storage/storageAccounts@2021-01-01",
					},
				},
			},
			expected: false,
		},
		{
			name: "Radius type with wrong API version",
			template: map[string]any{
				"resources": map[string]any{
					"app": map[string]any{
						"type": "Radius.Core/applications@2023-10-01-preview",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasOnlyRadiusResourceTypes(tt.template)
			require.Equal(t, tt.expected, result)
		})
	}
}

func Test_GetDeprecatedResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template map[string]any
		expected []DeprecatedResource
	}{
		{
			name:     "Nil template",
			template: nil,
			expected: nil,
		},
		{
			name:     "Empty template",
			template: map[string]any{},
			expected: nil,
		},
		{
			name: "Template with empty resources map",
			template: map[string]any{
				"resources": map[string]any{},
			},
			expected: nil,
		},
		{
			name: "Deprecated type with a known replacement",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "my-container",
					},
				},
			},
			expected: []DeprecatedResource{
				{
					FullType:    "Applications.Core/containers@2023-10-01-preview",
					Replacement: "Radius.Compute/containers@2025-08-01-preview",
				},
			},
		},
		{
			name: "Deprecated type without a known replacement",
			template: map[string]any{
				"resources": map[string]any{
					"store": map[string]any{
						"type": "Applications.Dapr/stateStores@2023-10-01-preview",
						"name": "my-store",
					},
				},
			},
			expected: []DeprecatedResource{
				{FullType: "Applications.Dapr/stateStores@2023-10-01-preview"},
			},
		},
		{
			name: "Deprecated type matching is case-insensitive",
			template: map[string]any{
				"resources": map[string]any{
					"gateway": map[string]any{
						"type": "applications.core/GATEWAYS@2023-10-01-PREVIEW",
						"name": "my-gateway",
					},
				},
			},
			expected: []DeprecatedResource{
				{
					FullType:    "applications.core/GATEWAYS@2023-10-01-PREVIEW",
					Replacement: "Radius.Compute/routes@2025-08-01-preview",
				},
			},
		},
		{
			name: "Applications type with a different API version is not deprecated",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": "Applications.Core/containers@2025-08-01-preview",
						"name": "my-container",
					},
				},
			},
			expected: nil,
		},
		{
			name: "Radius namespace type is not deprecated",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": "Radius.Compute/containers@2025-08-01-preview",
						"name": "my-container",
					},
				},
			},
			expected: nil,
		},
		{
			name: "Non-Radius type with the deprecated API version is not flagged",
			template: map[string]any{
				"resources": map[string]any{
					"storage": map[string]any{
						"type": "Microsoft.Storage/storageAccounts@2023-10-01-preview",
						"name": "my-storage",
					},
				},
			},
			expected: nil,
		},
		{
			name: "Resource type without an API version is not flagged",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": "Applications.Core/containers",
						"name": "my-container",
					},
				},
			},
			expected: nil,
		},
		{
			name: "Repeated resources of the same type are reported once",
			template: map[string]any{
				"resources": map[string]any{
					"containera": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "container-a",
					},
					"containerb": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "container-b",
					},
				},
			},
			expected: []DeprecatedResource{
				{
					FullType:    "Applications.Core/containers@2023-10-01-preview",
					Replacement: "Radius.Compute/containers@2025-08-01-preview",
				},
			},
		},
		{
			name: "Deprecated resource inside a Bicep module is detected",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"type": "Applications.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
					"module": map[string]any{
						"type": "Microsoft.Resources/deployments",
						"name": "my-module",
						"properties": map[string]any{
							"template": map[string]any{
								"resources": map[string]any{
									"app": map[string]any{
										"type": "Applications.Core/applications@2023-10-01-preview",
										"name": "my-app",
									},
								},
							},
						},
					},
				},
			},
			expected: []DeprecatedResource{
				{
					FullType:    "Applications.Core/applications@2023-10-01-preview",
					Replacement: "Radius.Core/applications@2025-08-01-preview",
				},
				{
					FullType:    "Applications.Core/environments@2023-10-01-preview",
					Replacement: "Radius.Core/environments@2025-08-01-preview",
				},
			},
		},
		{
			name: "Deprecated resource inside a nested module and an array-shaped template is detected",
			template: map[string]any{
				"resources": map[string]any{
					"outer": map[string]any{
						"type": "Microsoft.Resources/deployments",
						"name": "outer-module",
						"properties": map[string]any{
							"template": map[string]any{
								"resources": []any{
									map[string]any{
										"type": "Microsoft.Resources/deployments",
										"name": "inner-module",
										"properties": map[string]any{
											"template": map[string]any{
												"resources": map[string]any{
													"container": map[string]any{
														"type": "Applications.Core/containers@2023-10-01-preview",
														"name": "my-container",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: []DeprecatedResource{
				{
					FullType:    "Applications.Core/containers@2023-10-01-preview",
					Replacement: "Radius.Compute/containers@2025-08-01-preview",
				},
			},
		},
		{
			name: "Mixed resources are reported in sorted order",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "my-container",
					},
					"app": map[string]any{
						"type": "Applications.Core/applications@2023-10-01-preview",
						"name": "my-app",
					},
					"extender": map[string]any{
						"type": "Applications.Core/extenders@2023-10-01-preview",
						"name": "my-extender",
					},
					"modern": map[string]any{
						"type": "Radius.Core/environments@2025-08-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: []DeprecatedResource{
				{
					FullType:    "Applications.Core/applications@2023-10-01-preview",
					Replacement: "Radius.Core/applications@2025-08-01-preview",
				},
				{
					FullType:    "Applications.Core/containers@2023-10-01-preview",
					Replacement: "Radius.Compute/containers@2025-08-01-preview",
				},
				{FullType: "Applications.Core/extenders@2023-10-01-preview"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, GetDeprecatedResources(tt.template))
		})
	}
}

func Test_FormatDeprecationWarning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		deprecated []DeprecatedResource
		expected   string
	}{
		{
			name:       "No deprecated resources returns an empty string",
			deprecated: nil,
			expected:   "",
		},
		{
			name: "Resource with a replacement names the replacement type",
			deprecated: []DeprecatedResource{
				{
					FullType:    "Applications.Core/containers@2023-10-01-preview",
					Replacement: "Radius.Compute/containers@2025-08-01-preview",
				},
			},
			expected: "WARNING: The following resource types use the deprecated Applications.* namespace with API version 2023-10-01-preview and will be removed in a future release:\n" +
				"  - Applications.Core/containers@2023-10-01-preview: use Radius.Compute/containers@2025-08-01-preview instead.\n",
		},
		{
			name: "Resource without a replacement falls back to recipe pack guidance",
			deprecated: []DeprecatedResource{
				{FullType: "Applications.Core/extenders@2023-10-01-preview"},
			},
			expected: "WARNING: The following resource types use the deprecated Applications.* namespace with API version 2023-10-01-preview and will be removed in a future release:\n" +
				"  - Applications.Core/extenders@2023-10-01-preview: no direct replacement. Define an equivalent resource type and register a recipe pack that provides it to your environment.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, FormatDeprecationWarning(tt.deprecated))
		})
	}
}

// Test_GetDeprecatedResources_IsDeterministic guards the stable ordering and spelling of the
// warning. Resource types are case-insensitive, so a template may declare the same type with
// different casing. De-duplication keeps the first spelling encountered, which made the rendered
// output vary between runs while map iteration order was unsorted.
func Test_GetDeprecatedResources_IsDeterministic(t *testing.T) {
	t.Parallel()

	template := map[string]any{
		"resources": map[string]any{
			"containerUpper": map[string]any{
				"type": "Applications.Core/containers@2023-10-01-preview",
				"name": "container-upper",
			},
			"containerLower": map[string]any{
				"type": "applications.core/CONTAINERS@2023-10-01-preview",
				"name": "container-lower",
			},
			"app": map[string]any{
				"type": "Applications.Core/applications@2023-10-01-preview",
				"name": "my-app",
			},
			"module": map[string]any{
				"type": "Microsoft.Resources/deployments",
				"name": "my-module",
				"properties": map[string]any{
					"template": map[string]any{
						"resources": map[string]any{
							"gatewayA": map[string]any{
								"type": "Applications.Core/gateways@2023-10-01-preview",
								"name": "gateway-a",
							},
							"gatewayB": map[string]any{
								"type": "APPLICATIONS.CORE/GATEWAYS@2023-10-01-preview",
								"name": "gateway-b",
							},
						},
					},
				},
			},
		},
	}

	// Pin the exact spelling that wins de-duplication rather than comparing runs
	// against each other. Sorted key traversal makes the lowest-sorting symbolic
	// name win ("containerLower" over "containerUpper", "gatewayA" over
	// "gatewayB"), and the authored casing is preserved rather than canonicalized.
	expected := []string{
		"Applications.Core/applications@2023-10-01-preview",
		"Applications.Core/gateways@2023-10-01-preview",
		"applications.core/CONTAINERS@2023-10-01-preview",
	}

	// Repeat enough times to reliably surface randomized map iteration order.
	for range 100 {
		actual := GetDeprecatedResources(template)
		require.Len(t, actual, 3, "the two casing variants of each type should collapse to one entry")

		fullTypes := make([]string, 0, len(actual))
		for _, resource := range actual {
			fullTypes = append(fullTypes, resource.FullType)
		}
		require.Equal(t, expected, fullTypes)
	}
}

func Test_GetDeprecatedResources_MalformedTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template map[string]any
	}{
		{
			name: "Resources value is not a map or array",
			template: map[string]any{
				"resources": "not-a-collection",
			},
		},
		{
			name: "Resource entry is not a map",
			template: map[string]any{
				"resources": map[string]any{
					"bad": "not-a-map",
				},
			},
		},
		{
			name: "Array entry is not a map",
			template: map[string]any{
				"resources": []any{"not-a-map", 42, nil},
			},
		},
		{
			name: "Resource type is not a string",
			template: map[string]any{
				"resources": map[string]any{
					"bad": map[string]any{"type": 42},
				},
			},
		},
		{
			name: "Module properties are not a map",
			template: map[string]any{
				"resources": map[string]any{
					"module": map[string]any{
						"type":       "Microsoft.Resources/deployments",
						"properties": "not-a-map",
					},
				},
			},
		},
		{
			name: "Module template is not a map",
			template: map[string]any{
				"resources": map[string]any{
					"module": map[string]any{
						"type": "Microsoft.Resources/deployments",
						"properties": map[string]any{
							"template": "not-a-map",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Malformed input must be skipped rather than panic. A panic would
			// fail the test on its own, so no NotPanics wrapper is needed.
			require.Empty(t, GetDeprecatedResources(tt.template))
		})
	}
}

func Test_GetEnvironmentResources(t *testing.T) {
	t.Parallel()

	t.Run("Returns only Radius.Core environment resources", func(t *testing.T) {
		t.Parallel()

		radiusEnv := map[string]any{
			"type": "Radius.Core/environments@2025-08-01-preview",
			"name": "my-env",
		}
		template := map[string]any{
			"resources": map[string]any{
				"env": radiusEnv,
				"legacyEnv": map[string]any{
					"type": "Applications.Core/environments@2023-10-01-preview",
					"name": "legacy-env",
				},
				"app": map[string]any{
					"type": "Radius.Core/applications@2025-08-01-preview",
					"name": "my-app",
				},
			},
		}

		require.Equal(t, []map[string]any{radiusEnv}, GetEnvironmentResources(template))
	})

	t.Run("Returns nil when no environment resources are present", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, GetEnvironmentResources(nil))
		require.Nil(t, GetEnvironmentResources(map[string]any{}))
		require.Nil(t, GetEnvironmentResources(map[string]any{
			"resources": map[string]any{
				"app": map[string]any{"type": "Radius.Core/applications@2025-08-01-preview"},
			},
		}))
	})

	t.Run("Environment resources inside modules are intentionally not returned", func(t *testing.T) {
		t.Parallel()

		// Environment detection is deliberately top-level only, because it drives deployment
		// behavior. Only the deprecation scan recurses into modules.
		template := map[string]any{
			"resources": map[string]any{
				"module": map[string]any{
					"type": "Microsoft.Resources/deployments",
					"properties": map[string]any{
						"template": map[string]any{
							"resources": map[string]any{
								"env": map[string]any{"type": "Radius.Core/environments@2025-08-01-preview"},
							},
						},
					},
				},
			},
		}

		require.Nil(t, GetEnvironmentResources(template))
		require.False(t, ContainsEnvironmentResource(template))
	})
}
