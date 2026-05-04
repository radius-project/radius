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
