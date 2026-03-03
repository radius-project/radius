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
		name                           string
		template                       map[string]any
		expectedContainsEnvResource    bool
		expectedDeprecatedResources    []string
		expectedDeprecatedResourcesNil bool
	}{
		{
			name:                           "Nil template",
			template:                       nil,
			expectedContainsEnvResource:    false,
			expectedDeprecatedResources:    nil,
			expectedDeprecatedResourcesNil: true,
		},
		{
			name:                           "Empty template",
			template:                       map[string]any{},
			expectedContainsEnvResource:    false,
			expectedDeprecatedResources:    nil,
			expectedDeprecatedResourcesNil: true,
		},
		{
			name: "Template with missing resources field",
			template: map[string]any{
				"parameters": map[string]any{},
			},
			expectedContainsEnvResource:    false,
			expectedDeprecatedResources:    nil,
			expectedDeprecatedResourcesNil: true,
		},
		{
			name: "Template with empty resources map",
			template: map[string]any{
				"resources": map[string]any{},
			},
			expectedContainsEnvResource:    false,
			expectedDeprecatedResources:    []string{},
			expectedDeprecatedResourcesNil: false,
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
			expectedContainsEnvResource:    true,
			expectedDeprecatedResources:    []string{"Applications.Core/environments@2023-10-01-preview"},
			expectedDeprecatedResourcesNil: false,
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
			expectedContainsEnvResource:    true,
			expectedDeprecatedResources:    []string{},
			expectedDeprecatedResourcesNil: false,
		},
		{
			name: "Template with multiple resources - mixed deprecated and non-deprecated",
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
			expectedDeprecatedResources: []string{
				"Applications.Core/applications@2023-10-01-preview",
				"Applications.Core/containers@2023-10-01-preview",
			},
			expectedDeprecatedResourcesNil: false,
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
			expectedContainsEnvResource:    false,
			expectedDeprecatedResources:    nil,
			expectedDeprecatedResourcesNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InspectTemplateResources(tt.template)
			require.Equal(t, tt.expectedContainsEnvResource, result.ContainsEnvironmentResource)
			if tt.expectedDeprecatedResourcesNil {
				require.Nil(t, result.DeprecatedResources)
			} else {
				require.ElementsMatch(t, tt.expectedDeprecatedResources, result.DeprecatedResources)
			}
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

func Test_GetDeprecatedResources(t *testing.T) {
	tests := []struct {
		name     string
		template map[string]any
		expected []string
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
			name: "Template with missing resources field",
			template: map[string]any{
				"parameters": map[string]any{},
			},
			expected: nil,
		},
		{
			name: "Template with empty resources map",
			template: map[string]any{
				"resources": map[string]any{},
			},
			expected: []string{},
		},
		{
			name: "Template with deprecated Applications.Core resource",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "my-container",
					},
				},
			},
			expected: []string{"Applications.Core/containers@2023-10-01-preview"},
		},
		{
			name: "Template with deprecated Applications.Core resource - case insensitive",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": "applications.core/containers@2023-10-01-preview",
						"name": "my-container",
					},
				},
			},
			expected: []string{"applications.core/containers@2023-10-01-preview"},
		},
		{
			name: "Template with multiple deprecated Applications resources",
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
					"env": map[string]any{
						"type": "Applications.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: []string{
				"Applications.Core/applications@2023-10-01-preview",
				"Applications.Core/containers@2023-10-01-preview",
				"Applications.Core/environments@2023-10-01-preview",
			},
		},
		{
			name: "Template with non-deprecated Radius.Core resource",
			template: map[string]any{
				"resources": map[string]any{
					"env": map[string]any{
						"type": "Radius.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: []string{},
		},
		{
			name: "Template with mixed deprecated and non-deprecated resources",
			template: map[string]any{
				"resources": map[string]any{
					"deprecated": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "my-container",
					},
					"nonDeprecated": map[string]any{
						"type": "Radius.Core/environments@2023-10-01-preview",
						"name": "my-env",
					},
				},
			},
			expected: []string{"Applications.Core/containers@2023-10-01-preview"},
		},
		{
			name: "Template with Applications resource but different API version",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": "Applications.Core/containers@2024-01-01",
						"name": "my-container",
					},
				},
			},
			expected: []string{},
		},
		{
			name: "Template with Applications resource and similar but different API version suffix",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview-v2",
						"name": "my-container",
					},
				},
			},
			expected: []string{},
		},
		{
			name: "Template with invalid resources format (array instead of map)",
			template: map[string]any{
				"resources": []any{
					map[string]any{
						"type": "Applications.Core/containers@2023-10-01-preview",
						"name": "my-container",
					},
				},
			},
			expected: nil,
		},
		{
			name: "Template with invalid resource format (not a map)",
			template: map[string]any{
				"resources": map[string]any{
					"container": "not a map",
				},
			},
			expected: []string{},
		},
		{
			name: "Template with resource missing type field",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"name": "my-container",
					},
				},
			},
			expected: []string{},
		},
		{
			name: "Template with resource type not a string",
			template: map[string]any{
				"resources": map[string]any{
					"container": map[string]any{
						"type": 123,
						"name": "my-container",
					},
				},
			},
			expected: []string{},
		},
		{
			name: "Template with Applications.Dapr resource",
			template: map[string]any{
				"resources": map[string]any{
					"stateStore": map[string]any{
						"type": "Applications.Dapr/stateStores@2023-10-01-preview",
						"name": "my-statestore",
					},
				},
			},
			expected: []string{"Applications.Dapr/stateStores@2023-10-01-preview"},
		},
		{
			name: "Template with Applications.Datastores resource",
			template: map[string]any{
				"resources": map[string]any{
					"redis": map[string]any{
						"type": "Applications.Datastores/redisCaches@2023-10-01-preview",
						"name": "my-redis",
					},
				},
			},
			expected: []string{"Applications.Datastores/redisCaches@2023-10-01-preview"},
		},
		{
			name: "Template with Applications.Messaging resource",
			template: map[string]any{
				"resources": map[string]any{
					"queue": map[string]any{
						"type": "Applications.Messaging/rabbitMQQueues@2023-10-01-preview",
						"name": "my-queue",
					},
				},
			},
			expected: []string{"Applications.Messaging/rabbitMQQueues@2023-10-01-preview"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDeprecatedResources(tt.template)
			if tt.expected == nil {
				require.Nil(t, result)
			} else {
				require.ElementsMatch(t, tt.expected, result)
			}
		})
	}
}
