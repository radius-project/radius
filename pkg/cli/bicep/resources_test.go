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
