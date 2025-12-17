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

package resourceutil

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

const (
	TestResourceType  = "Applications.Test/testResources"
	TestEnvironmentID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"
	TestApplicationID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"
	TestResourceID    = "/planes/radius/local/resourceGroups/radius-test-rg/providers/MyResources.Test/testResources/tr"
)

type PropertiesTestResource struct {
	v1.BaseResource
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

func (p *PropertiesTestResource) ResourceMetadata() rpv1.BasicResourcePropertiesAdapter {
	return nil
}

func (p *PropertiesTestResource) ApplyDeploymentOutput(deploymentOutput rpv1.DeploymentOutput) error {
	return nil
}

func (p *PropertiesTestResource) OutputResources() []rpv1.OutputResource {
	return nil
}

type InvalidResourceForAll struct {
	Properties map[string]any `json:"properties"`
	BadField   func()         `json:"badField"` // Functions cannot be marshaled
}

func TestGetPropertiesFromResource(t *testing.T) {
	tests := []struct {
		name        string
		resource    *PropertiesTestResource
		expected    map[string]any
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid properties",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"Application": TestApplicationID,
					"Environment": TestEnvironmentID,
				},
			},
			expected: map[string]any{
				"Application": TestApplicationID,
				"Environment": TestEnvironmentID,
			},
			expectError: false,
		},
		{
			name: "Empty properties",
			resource: &PropertiesTestResource{
				Properties: nil,
			},
			expected:    map[string]any{},
			expectError: false,
		},
		{
			name: "Invalid JSON",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"key": func() {}, // Functions cannot be marshaled to JSON
				},
			},
			expected:    nil,
			expectError: true,
			errorMsg:    errMarshalResource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			properties, err := GetPropertiesFromResource(tt.resource)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, properties)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, properties)
				require.Equal(t, tt.expected, properties)
			}
		})
	}
}

// InvalidTestResource is a test resource with invalid properties type.
type InvalidTestResource struct {
	v1.BaseResource
	Name string `json:"properties"`
}

func (p *InvalidTestResource) ResourceMetadata() rpv1.BasicResourcePropertiesAdapter {
	return nil
}

func (p *InvalidTestResource) ApplyDeploymentOutput(deploymentOutput rpv1.DeploymentOutput) error {
	return nil
}

func (p *InvalidTestResource) OutputResources() []rpv1.OutputResource {
	return nil
}

func TestGetPropertiesFromResource_MissingProperties(t *testing.T) {
	testResource := &InvalidTestResource{
		Name: "test-resource",
	}

	properties, err := GetPropertiesFromResource(testResource)
	require.Error(t, err)
	require.Nil(t, properties)
	require.Contains(t, err.Error(), errUnmarshalResourceProperties)
}

func TestGetConnectionNameandSourceIDs(t *testing.T) {
	tests := []struct {
		name        string
		resource    *PropertiesTestResource
		expected    map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid connections with multiple sources",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"connections": map[string]any{
						"database": map[string]any{
							"source": "/planes/radius/local/resourceGroups/rg/providers/MyResources.Datastores/sqlDatabases/db1",
						},
						"redis": map[string]any{
							"source": "/planes/radius/local/resourceGroups/rg/providers/MyResources.Caches/redisCaches/cache1",
						},
					},
				},
			},
			expected: map[string]string{
				"database": "/planes/radius/local/resourceGroups/rg/providers/MyResources.Datastores/sqlDatabases/db1",
				"redis":    "/planes/radius/local/resourceGroups/rg/providers/MyResources.Caches/redisCaches/cache1",
			},
			expectError: false,
		},

		{
			name: "Single valid connection",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"connections": map[string]any{
						"storage": map[string]any{
							"source": "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/storageAccounts/storage1",
						},
					},
				},
			},
			expected: map[string]string{
				"storage": "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/storageAccounts/storage1",
			},
			expectError: false,
		},
		{
			name: "Empty connections map",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"connections": map[string]any{},
				},
			},
			expected:    map[string]string{},
			expectError: false,
		},
		{
			name: "No connections property",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"application": TestApplicationID,
					"environment": TestEnvironmentID,
				},
			},
			expected:    map[string]string{},
			expectError: false,
		},
		{
			name: "Nil properties",
			resource: &PropertiesTestResource{
				Properties: nil,
			},
			expected:    map[string]string{},
			expectError: false,
		},
		{
			name: "Connections is nil",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"connections": nil,
				},
			},
			expected:    map[string]string{},
			expectError: false,
		},
		{
			name: "Invalid connections type (not a map)",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"connections": "invalid-string",
				},
			},
			expected:    nil,
			expectError: true,
			errorMsg:    "failed to get connections from resource properties",
		},
		{
			name: "Missing source field in connection",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"connections": map[string]any{
						"database": map[string]any{
							"type": "sql",
						},
					},
				},
			},
			expected:    nil,
			expectError: true,
			errorMsg:    "source not found in connection \"database\"",
		},

		{
			name: "Invalid resource ID format",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"connections": map[string]any{
						"database": map[string]any{
							"source": "invalid-resource-id",
						},
					},
				},
			},
			expected:    nil,
			expectError: true,
			errorMsg:    "invalid resource ID in connection database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetConnectionNameandSourceIDs(tt.resource)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, result)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetConnectionNameandSourceIDs_InvalidJSONMarshaling(t *testing.T) {
	// Test case where the resource itself cannot be marshaled to JSON
	type InvalidResource struct {
		Properties map[string]any `json:"properties"`
		BadField   func()         `json:"badField"` // Functions cannot be marshaled
	}

	resource := &InvalidResource{
		Properties: map[string]any{
			"connections": map[string]any{
				"database": map[string]any{
					"source": "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/sqlDatabases/db1",
				},
			},
		},
		BadField: func() {}, // This will cause JSON marshaling to fail
	}

	result, err := GetConnectionNameandSourceIDs(resource)
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), errMarshalResource)
}

func TestMarshalAndUnmarshalResource(t *testing.T) {
	t.Run("Valid marshal and unmarshal to same struct type", func(t *testing.T) {
		source := &PropertiesTestResource{
			ID:   TestResourceID,
			Name: "test-resource",
			Type: "MyResources.Test/testResources",
			Properties: map[string]any{
				"host": "localhost",
				"port": 5432,
			},
		}

		result, err := GetAllPropertiesFromResource(source)
		require.NoError(t, err)
		require.Equal(t, TestResourceID, result.ID)
		require.Equal(t, "test-resource", result.Name)
		require.Equal(t, "MyResources.Test/testResources", result.Type)
		require.Equal(t, "localhost", result.Properties["host"])
		require.Equal(t, float64(5432), result.Properties["port"]) // JSON numbers are float64
	})

	t.Run("Unmarshal properties through GetPropertiesFromResource", func(t *testing.T) {
		source := &PropertiesTestResource{
			ID:   TestResourceID,
			Name: "test-resource",
			Type: "MyResources.Test/testResources",
			Properties: map[string]any{
				"key":     "value",
				"number":  123,
				"boolean": true,
				"nested": map[string]any{
					"inner": "value",
				},
			},
		}

		properties, err := GetPropertiesFromResource(source)
		require.NoError(t, err)
		require.Equal(t, "value", properties["key"])
		require.Equal(t, float64(123), properties["number"]) // JSON numbers are float64
		require.Equal(t, true, properties["boolean"])
		require.IsType(t, map[string]any{}, properties["nested"])
		nested := properties["nested"].(map[string]any)
		require.Equal(t, "value", nested["inner"])
	})

	t.Run("Marshal error through GetPropertiesFromResource", func(t *testing.T) {
		source := &InvalidResourceForAll{
			Properties: map[string]any{"key": "value"},
			BadField:   func() {}, // Functions cannot be marshaled
		}

		_, err := GetPropertiesFromResource(source)
		require.Error(t, err)
		require.Contains(t, err.Error(), errMarshalResource)
	})

	t.Run("Unmarshal error through GetAllPropertiesFromResource", func(t *testing.T) {
		source := &InvalidResourceForAll{
			Properties: map[string]any{"key": "value"},
			BadField:   func() {}, // Functions cannot be marshaled
		}

		_, err := GetAllPropertiesFromResource(source)
		require.Error(t, err)
		require.Contains(t, err.Error(), errMarshalResource)
	})

	t.Run("Test data type preservation", func(t *testing.T) {
		source := &PropertiesTestResource{
			ID:   TestResourceID,
			Name: "test-resource",
			Type: "MyResources.Test/testResources",
			Properties: map[string]any{
				"string":  "value",
				"int":     42,
				"float":   3.14,
				"bool":    true,
				"null":    nil,
				"array":   []string{"item1", "item2"},
				"object": map[string]any{
					"nested": "value",
				},
			},
		}

		result, err := GetAllPropertiesFromResource(source)
		require.NoError(t, err)

		// Check that data types are preserved correctly through JSON marshal/unmarshal
		require.Equal(t, "value", result.Properties["string"])
		require.Equal(t, float64(42), result.Properties["int"]) // JSON converts all numbers to float64
		require.Equal(t, 3.14, result.Properties["float"])
		require.Equal(t, true, result.Properties["bool"])
		require.Nil(t, result.Properties["null"])
		
		// Arrays become []any after JSON processing
		array := result.Properties["array"].([]any)
		require.Equal(t, "item1", array[0])
		require.Equal(t, "item2", array[1])
		
		// Objects remain as map[string]any
		object := result.Properties["object"].(map[string]any)
		require.Equal(t, "value", object["nested"])
	})
}

func TestGetAllPropertiesFromResource(t *testing.T) {
	tests := []struct {
		name        string
		resource    any
		expected    *ResourceMetadata
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid resource with properties",
			resource: &PropertiesTestResource{
				ID:   TestResourceID,
				Name: "tr",
				Type: "MyResources.Test/testResources",
				Properties: map[string]any{
					"Application": TestApplicationID,
					"Environment": TestEnvironmentID,
					"host":        "localhost",
					"port":        float64(5432),
				},
			},
			expected: &ResourceMetadata{
				ID:   TestResourceID,
				Name: "tr",
				Type: "MyResources.Test/testResources",
				Properties: map[string]any{
					"Application": TestApplicationID,
					"Environment": TestEnvironmentID,
					"host":        "localhost",
					"port":        float64(5432),
				},
			},
			expectError: false,
		},
		{
			name: "Resource with empty properties",
			resource: &PropertiesTestResource{
				ID:         TestResourceID,
				Name:       "tr",
				Type:       "MyResources.Test/testResources",
				Properties: nil,
			},
			expected: &ResourceMetadata{
				ID:         TestResourceID,
				Name:       "tr",
				Type:       "MyResources.Test/testResources",
				Properties: map[string]any{},
			},
			expectError: false,
		},
		{
			name: "Resource with missing fields",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"host": "localhost",
				},
			},
			expected: &ResourceMetadata{
				ID:         "",
				Name:       "",
				Type:       "",
				Properties: map[string]any{
					"host": "localhost",
				},
			},
			expectError: false,
		},
		{
			name: "Resource with complex nested properties",
			resource: &PropertiesTestResource{
				ID:   TestResourceID,
				Name: "tr",
				Type: "MyResources.Test/testResources",
				Properties: map[string]any{
					"Application": TestApplicationID,
					"Environment": TestEnvironmentID,
					"config": map[string]any{
						"nested": "value",
						"array":  []string{"item1", "item2"},
					},
					"enabled": true,
				},
			},
			expected: &ResourceMetadata{
				ID:   TestResourceID,
				Name: "tr",
				Type: "MyResources.Test/testResources",
				Properties: map[string]any{
					"Application": TestApplicationID,
					"Environment": TestEnvironmentID,
					"config": map[string]any{
						"nested": "value",
						"array":  []any{"item1", "item2"}, // JSON unmarshaling converts to []any
					},
					"enabled": true,
				},
			},
			expectError: false,
		},
		{
			name: "Resource with unmarshalable data",
			resource: &InvalidResourceForAll{
				Properties: map[string]any{
					"host": "localhost",
				},
				BadField: func() {}, // This will cause JSON marshaling to fail
			},
			expected:    nil,
			expectError: true,
			errorMsg:    errMarshalResource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetAllPropertiesFromResource(tt.resource)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, result)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}
