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

import (
	"testing"

	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/stretchr/testify/require"
)

func Test_DynamicResource_Status(t *testing.T) {
	tests := []struct {
		name     string
		resource DynamicResource
		want     map[string]any
	}{
		{
			name:     "nil properties returns empty map",
			resource: DynamicResource{},
			want:     map[string]any{},
		},
		{
			name: "no status in properties returns empty map",
			resource: DynamicResource{
				Properties: map[string]any{
					"otherField": "value",
				},
			},
			want: map[string]any{},
		},
		{
			name: "non-map status in properties returns empty map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": "invalid-string-status",
				},
			},
			want: map[string]any{},
		},
		{
			name: "valid status in properties returns status map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"phase":   "Ready",
						"message": "Resource is ready",
					},
				},
			},
			want: map[string]any{
				"phase":   "Ready",
				"message": "Resource is ready",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resource.Status()
			require.Equal(t, tt.want, got)

			// Verify that calling Status() initializes properties if nil
			require.NotNil(t, tt.resource.Properties)

			// Verify that status field is properly set in properties
			status, ok := tt.resource.Properties["status"]
			require.True(t, ok)
			require.IsType(t, map[string]any{}, status)
		})
	}
}

func Test_DynamicResource_GetRecipe(t *testing.T) {
	tests := []struct {
		name     string
		resource DynamicResource
		want     *portableresources.ResourceRecipe
	}{
		{
			name:     "nil properties returns empty recipe",
			resource: DynamicResource{},
			want:     &portableresources.ResourceRecipe{Name: "default"},
		},
		{
			name: "no recipe in properties returns empty recipe",
			resource: DynamicResource{
				Properties: map[string]any{
					"otherField": "value",
				},
			},
			want: &portableresources.ResourceRecipe{Name: "default"},
		},
		{
			name: "non-map recipe in properties returns empty recipe",
			resource: DynamicResource{
				Properties: map[string]any{
					"recipe": "invalid-string-recipe",
				},
			},
			want: &portableresources.ResourceRecipe{Name: "default"},
		},
		{
			name: "valid recipe in properties returns recipe",
			resource: DynamicResource{
				Properties: map[string]any{
					"recipe": map[string]any{
						"name": "test-recipe",
						"parameters": map[string]any{
							"param1": "value1",
						},
						"recipeStatus": "Succeeded",
					},
				},
			},
			want: &portableresources.ResourceRecipe{
				Name: "test-recipe",
				Parameters: map[string]any{
					"param1": "value1",
				},
				DeploymentStatus: "Succeeded",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resource.GetRecipe()
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_DynamicResource_SetRecipe(t *testing.T) {
	tests := []struct {
		name     string
		resource DynamicResource
		recipe   *portableresources.ResourceRecipe
		want     map[string]any
	}{
		{
			name:     "nil properties initialized when setting recipe",
			resource: DynamicResource{},
			recipe: &portableresources.ResourceRecipe{
				Name: "test-recipe",
				Parameters: map[string]any{
					"param1": "value1",
				},
				DeploymentStatus: "Succeeded",
			},
			want: map[string]any{
				"name": "test-recipe",
				"parameters": map[string]any{
					"param1": "value1",
				},
				"recipeStatus": "Succeeded",
			},
		},
		{
			name: "existing properties preserved when setting recipe",
			resource: DynamicResource{
				Properties: map[string]any{
					"otherField": "value",
				},
			},
			recipe: &portableresources.ResourceRecipe{
				Name: "test-recipe",
			},
			want: map[string]any{
				"name": "test-recipe",
			},
		},
		{
			name:     "setting nil recipe initializes empty recipe map",
			resource: DynamicResource{},
			recipe:   nil,
			want:     map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.resource.SetRecipe(tt.recipe)

			// Verify properties are initialized
			require.NotNil(t, tt.resource.Properties)

			// Get the recipe map from properties
			recipe, ok := tt.resource.Properties["recipe"]
			require.True(t, ok)

			// For nil recipe case, verify it's an empty map
			if tt.recipe == nil {
				require.Equal(t, map[string]any{}, recipe)
				return
			}

			// Verify the recipe map matches expected values
			recipeMap, ok := recipe.(map[string]any)
			require.True(t, ok)
			require.Equal(t, tt.want, recipeMap)
		})
	}
}

func Test_DynamicResourceBasicPropertiesAdapter_ApplicationID(t *testing.T) {
	tests := []struct {
		name     string
		resource DynamicResource
		want     string
	}{
		{
			name:     "nil properties returns empty string",
			resource: DynamicResource{},
			want:     "",
		},
		{
			name: "no application in properties returns empty string",
			resource: DynamicResource{
				Properties: map[string]any{
					"otherField": "value",
				},
			},
			want: "",
		},
		{
			name: "non-string application in properties returns empty string",
			resource: DynamicResource{
				Properties: map[string]any{
					"application": 123,
				},
			},
			want: "",
		},
		{
			name: "valid application in properties returns value",
			resource: DynamicResource{
				Properties: map[string]any{
					"application": "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/applications/test-app",
				},
			},
			want: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/applications/test-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &dynamicResourceBasicPropertiesAdapter{resource: &tt.resource}
			got := adapter.ApplicationID()
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_DynamicResourceBasicPropertiesAdapter_EnvironmentID(t *testing.T) {
	tests := []struct {
		name     string
		resource DynamicResource
		want     string
	}{
		{
			name:     "nil properties returns empty string",
			resource: DynamicResource{},
			want:     "",
		},
		{
			name: "no environment in properties returns empty string",
			resource: DynamicResource{
				Properties: map[string]any{
					"otherField": "value",
				},
			},
			want: "",
		},
		{
			name: "non-string environment in properties returns empty string",
			resource: DynamicResource{
				Properties: map[string]any{
					"environment": 123,
				},
			},
			want: "",
		},
		{
			name: "valid environment in properties returns value",
			resource: DynamicResource{
				Properties: map[string]any{
					"environment": "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/test-env",
				},
			},
			want: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/test-env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &dynamicResourceBasicPropertiesAdapter{resource: &tt.resource}
			got := adapter.EnvironmentID()
			require.Equal(t, tt.want, got)
		})
	}
}
