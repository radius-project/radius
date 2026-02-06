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

package frontend

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactField_SimpleField(t *testing.T) {
	// Test redacting a simple top-level field
	properties := map[string]any{
		"name":     "test-resource",
		"password": "secret123",
		"data":     map[string]any{"key": "value"},
	}

	redactField(properties, "password")

	require.Equal(t, "test-resource", properties["name"])
	require.Nil(t, properties["password"])
	require.NotNil(t, properties["data"])
}

func TestRedactField_DataField(t *testing.T) {
	// Test redacting the "data" field (common pattern for secrets)
	properties := map[string]any{
		"environment": "/planes/radius/local/resourcegroups/default/providers/Radius.Core/environments/test",
		"application": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/applications/test",
		"data": map[string]any{
			"password": map[string]any{
				"value":    "secret123",
				"encoding": "string",
			},
			"apiKey": map[string]any{
				"value": "my-api-key",
			},
		},
	}

	redactField(properties, "data")

	require.NotNil(t, properties["environment"])
	require.NotNil(t, properties["application"])
	require.Nil(t, properties["data"])
}

func TestRedactField_NonExistentField(t *testing.T) {
	// Test that redacting a non-existent field doesn't cause errors
	properties := map[string]any{
		"name":  "test-resource",
		"value": "test-value",
	}

	// Should not panic or error
	redactField(properties, "nonexistent")

	// Original fields should remain unchanged
	require.Equal(t, "test-resource", properties["name"])
	require.Equal(t, "test-value", properties["value"])
}

func TestRedactField_NilProperties(t *testing.T) {
	// Test that nil properties don't cause panic
	var properties map[string]any

	// Should not panic
	redactField(properties, "anyfield")
}

func TestRedactField_EmptyProperties(t *testing.T) {
	// Test redacting from empty properties
	properties := map[string]any{}

	redactField(properties, "password")

	require.Empty(t, properties)
}

func TestRedactField_MultipleFields(t *testing.T) {
	// Test redacting multiple fields sequentially
	properties := map[string]any{
		"name":     "test",
		"password": "secret",
		"apiKey":   "key123",
		"data":     "sensitive-data",
	}

	redactField(properties, "password")
	redactField(properties, "apiKey")
	redactField(properties, "data")

	require.Equal(t, "test", properties["name"])
	require.Nil(t, properties["password"])
	require.Nil(t, properties["apiKey"])
	require.Nil(t, properties["data"])
}

func TestRedactField_NestedFieldNotSupported(t *testing.T) {
	// Test that nested paths (with dots) are not currently supported
	// The redactField function currently only handles top-level fields
	properties := map[string]any{
		"config": map[string]any{
			"password": "secret",
		},
	}

	// Nested path - should not redact since we only support top-level
	redactField(properties, "config.password")

	// Config should still contain the password (nested redaction not supported)
	config, ok := properties["config"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "secret", config["password"])
}

func TestRedactField_FieldWithNilValue(t *testing.T) {
	// Test redacting a field that already has nil value
	properties := map[string]any{
		"name":     "test",
		"password": nil,
	}

	redactField(properties, "password")

	require.Equal(t, "test", properties["name"])
	require.Nil(t, properties["password"])
}

func TestRedactField_FieldWithDifferentTypes(t *testing.T) {
	// Test redacting fields of various types
	testCases := []struct {
		name       string
		value      any
		fieldName  string
		properties map[string]any
	}{
		{
			name:       "string field",
			fieldName:  "secret",
			properties: map[string]any{"secret": "password123"},
		},
		{
			name:       "map field",
			fieldName:  "data",
			properties: map[string]any{"data": map[string]any{"key": "value"}},
		},
		{
			name:       "slice field",
			fieldName:  "tokens",
			properties: map[string]any{"tokens": []string{"token1", "token2"}},
		},
		{
			name:       "int field",
			fieldName:  "pin",
			properties: map[string]any{"pin": 1234},
		},
		{
			name:       "bool field",
			fieldName:  "enabled",
			properties: map[string]any{"enabled": true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			redactField(tc.properties, tc.fieldName)
			require.Nil(t, tc.properties[tc.fieldName])
		})
	}
}
