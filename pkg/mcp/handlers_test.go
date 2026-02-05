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

package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterDiscoveryTools(t *testing.T) {
	server := NewServer()

	err := RegisterDiscoveryTools(server)
	require.NoError(t, err)

	tools := server.ListTools()
	
	// Verify all expected tools are registered
	expectedTools := []string{
		"discover_dependencies",
		"discover_services",
		"generate_resource_types",
		"generate_app_definition",
		"validate_app_definition",
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	for _, expected := range expectedTools {
		assert.True(t, toolNames[expected], "expected tool %s to be registered", expected)
	}
}

func TestDiscoverDependenciesHandler(t *testing.T) {
	handler := NewDiscoverDependenciesHandler()

	assert.Equal(t, "discover_dependencies", handler.Name())
	assert.NotEmpty(t, handler.Description())

	schema := handler.InputSchema()
	assert.NotEmpty(t, schema)

	// Verify schema is valid JSON
	var schemaMap map[string]interface{}
	err := json.Unmarshal(schema, &schemaMap)
	require.NoError(t, err)
	assert.Equal(t, "object", schemaMap["type"])
}

func TestDiscoverDependenciesHandler_MissingProjectPath(t *testing.T) {
	handler := NewDiscoverDependenciesHandler()

	_, err := handler.Handle(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "projectPath is required")
}

func TestDiscoverServicesHandler(t *testing.T) {
	handler := NewDiscoverServicesHandler()

	assert.Equal(t, "discover_services", handler.Name())
	assert.NotEmpty(t, handler.Description())

	schema := handler.InputSchema()
	assert.NotEmpty(t, schema)

	var schemaMap map[string]interface{}
	err := json.Unmarshal(schema, &schemaMap)
	require.NoError(t, err)
	assert.Equal(t, "object", schemaMap["type"])
}

func TestDiscoverServicesHandler_MissingProjectPath(t *testing.T) {
	handler := NewDiscoverServicesHandler()

	_, err := handler.Handle(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "projectPath is required")
}

func TestGenerateResourceTypesHandler(t *testing.T) {
	handler := NewGenerateResourceTypesHandler()

	assert.Equal(t, "generate_resource_types", handler.Name())
	assert.NotEmpty(t, handler.Description())

	schema := handler.InputSchema()
	assert.NotEmpty(t, schema)

	var schemaMap map[string]interface{}
	err := json.Unmarshal(schema, &schemaMap)
	require.NoError(t, err)
	assert.Equal(t, "object", schemaMap["type"])
}

func TestGenerateResourceTypesHandler_MissingDependencies(t *testing.T) {
	handler := NewGenerateResourceTypesHandler()

	_, err := handler.Handle(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dependencies is required")
}

func TestGenerateAppDefinitionHandler(t *testing.T) {
	handler := NewGenerateAppDefinitionHandler()

	assert.Equal(t, "generate_app_definition", handler.Name())
	assert.NotEmpty(t, handler.Description())

	schema := handler.InputSchema()
	assert.NotEmpty(t, schema)

	var schemaMap map[string]interface{}
	err := json.Unmarshal(schema, &schemaMap)
	require.NoError(t, err)
	assert.Equal(t, "object", schemaMap["type"])
}

func TestGenerateAppDefinitionHandler_MissingDiscoveryResult(t *testing.T) {
	handler := NewGenerateAppDefinitionHandler()

	_, err := handler.Handle(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "discoveryResult is required")
}

func TestGenerateAppDefinitionHandler_WithValidInput(t *testing.T) {
	handler := NewGenerateAppDefinitionHandler()

	input := `{
		"discoveryResult": {
			"projectPath": "/test/app",
			"services": [
				{
					"name": "api",
					"language": "javascript",
					"exposedPorts": [3000]
				}
			],
			"dependencies": [],
			"resourceTypes": []
		},
		"applicationName": "test-app",
		"includeComments": true
	}`

	result, err := handler.Handle(context.Background(), json.RawMessage(input))
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestValidateAppDefinitionHandler(t *testing.T) {
	handler := NewValidateAppDefinitionHandler()

	assert.Equal(t, "validate_app_definition", handler.Name())
	assert.NotEmpty(t, handler.Description())

	schema := handler.InputSchema()
	assert.NotEmpty(t, schema)

	var schemaMap map[string]interface{}
	err := json.Unmarshal(schema, &schemaMap)
	require.NoError(t, err)
	assert.Equal(t, "object", schemaMap["type"])
}

func TestValidateAppDefinitionHandler_MissingInput(t *testing.T) {
	handler := NewValidateAppDefinitionHandler()

	_, err := handler.Handle(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "either bicepContent or filePath is required")
}

func TestValidateAppDefinitionHandler_WithValidBicep(t *testing.T) {
	handler := NewValidateAppDefinitionHandler()

	validBicep := `extension radius

param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
  properties: {
    environment: environment
  }
}`

	input, _ := json.Marshal(map[string]interface{}{
		"bicepContent": validBicep,
	})

	result, err := handler.Handle(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestHandlerInputParsing(t *testing.T) {
	tests := []struct {
		name      string
		handler   ToolHandler
		input     string
		expectErr bool
	}{
		{
			name:      "discover_dependencies invalid json",
			handler:   NewDiscoverDependenciesHandler(),
			input:     `{invalid}`,
			expectErr: true,
		},
		{
			name:      "discover_services invalid json",
			handler:   NewDiscoverServicesHandler(),
			input:     `{invalid}`,
			expectErr: true,
		},
		{
			name:      "generate_resource_types invalid json",
			handler:   NewGenerateResourceTypesHandler(),
			input:     `{invalid}`,
			expectErr: true,
		},
		{
			name:      "generate_app_definition invalid json",
			handler:   NewGenerateAppDefinitionHandler(),
			input:     `{invalid}`,
			expectErr: true,
		},
		{
			name:      "validate_app_definition invalid json",
			handler:   NewValidateAppDefinitionHandler(),
			input:     `{invalid}`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.handler.Handle(context.Background(), json.RawMessage(tt.input))
			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "parsing input")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
