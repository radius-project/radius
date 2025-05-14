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
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
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

func Test_DynamicResource_OutputVariables(t *testing.T) {
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
			name: "no output variables in status returns empty map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"otherField": "value",
					},
				},
			},
			want: map[string]any{},
		},
		{
			name: "non-map output variables in status returns empty map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"outputVariables": "invalid-output-variables",
					},
				},
			},
			want: map[string]any{},
		},
		{
			name: "valid output variables in status returns output variables map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"outputVariables": map[string]any{
							"var1": "value1",
							"var2": "value2",
						},
					},
				},
			},
			want: map[string]any{
				"var1": "value1",
				"var2": "value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resource.OutputVariables()
			require.Equal(t, tt.want, got)

			if len(tt.want) > 0 {
				require.NotNil(t, tt.resource.Properties["status"])
				status, ok := tt.resource.Properties["status"].(map[string]any)
				require.True(t, ok)
				outputVars, ok := status["outputVariables"]
				require.True(t, ok)
				require.IsType(t, map[string]any{}, outputVars)
			}
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

func Test_DynamicResource_GetComputedValues(t *testing.T) {
	tests := []struct {
		name     string
		resource DynamicResource
		want     map[string]any
	}{
		{
			name: "valid computedValues returns map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"computedValues": map[string]any{
							"foo": "bar",
							"num": 42,
						},
					},
				},
			},
			want: map[string]any{"foo": "bar", "num": 42},
		},
		{
			name: "empty status returns empty map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{},
				},
			},
			want: map[string]any{},
		},
		{
			name:     "nil properties returns empty map",
			resource: DynamicResource{},
			want:     map[string]any{},
		},
		{
			name: "non-map computedValues returns empty map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"computedValues": "invalid",
					},
				},
			},
			want: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resource.GetComputedValues()
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_DynamicResource_GetSecrets(t *testing.T) {
	tests := []struct {
		name     string
		resource DynamicResource
		want     map[string]rpv1.SecretValueReference
	}{
		{
			name: "valid secrets returns map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"secrets": map[string]any{
							"password": rpv1.SecretValueReference{Value: "s3cr3t"},
							"token":    rpv1.SecretValueReference{Value: "tok123"},
						},
					},
				},
			},
			want: map[string]rpv1.SecretValueReference{
				"password": {Value: "s3cr3t"},
				"token":    {Value: "tok123"},
			},
		},
		{
			name: "string secrets are ignored",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"secrets": map[string]any{
							"password": "s3cr3t",
							"token":    rpv1.SecretValueReference{Value: "tok123"},
						},
					},
				},
			},
			want: map[string]rpv1.SecretValueReference{
				"token": {Value: "tok123"},
			},
		},
		{
			name: "empty status returns empty map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{},
				},
			},
			want: map[string]rpv1.SecretValueReference{},
		},
		{
			name:     "nil properties returns empty map",
			resource: DynamicResource{},
			want:     map[string]rpv1.SecretValueReference{},
		},
		{
			name: "non-map secrets returns empty map",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"secrets": "invalid",
					},
				},
			},
			want: map[string]rpv1.SecretValueReference{},
		},
		{
			name: "JSON-marshaled secrets are converted correctly",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"secrets": map[string]any{
							"password": map[string]any{"Value": "s3cr3t"},
							"token":    map[string]any{"Value": "tok123"},
						},
					},
				},
			},
			want: map[string]rpv1.SecretValueReference{
				"password": {Value: "s3cr3t"},
				"token":    {Value: "tok123"},
			},
		},
		{
			name: "mixed format secrets (direct struct and JSON-marshaled)",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"secrets": map[string]any{
							"password": rpv1.SecretValueReference{Value: "s3cr3t"},
							"token":    map[string]any{"Value": "tok123"},
						},
					},
				},
			},
			want: map[string]rpv1.SecretValueReference{
				"password": {Value: "s3cr3t"},
				"token":    {Value: "tok123"},
			},
		},
		{
			name: "malformed JSON-marshaled secrets are ignored",
			resource: DynamicResource{
				Properties: map[string]any{
					"status": map[string]any{
						"secrets": map[string]any{
							"password":   map[string]any{"Value": "s3cr3t"},
							"badSecret1": map[string]any{"WrongField": "value"},
							"badSecret2": map[string]any{"Value": 123}, // non-string value
							"goodSecret": rpv1.SecretValueReference{Value: "good"},
						},
					},
				},
			},
			want: map[string]rpv1.SecretValueReference{
				"password":   {Value: "s3cr3t"},
				"goodSecret": {Value: "good"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resource.GetSecrets()
			require.Equal(t, tt.want, got)
		})
	}
}
