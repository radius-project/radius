/*
Copyright 2026 The Radius Authors.

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

package schema

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestValidateResourceRequestAgainstSchema(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tls": map[string]any{"type": "string", "enum": []any{"required", "optional"}},
			"password": map[string]any{
				"type": "string", "minLength": 8, "pattern": "^[a-z]+$", "x-radius-sensitive": true,
			},
			"credentials": map[string]any{
				"type": "object", "x-radius-sensitive": true,
				"properties": map[string]any{"token": map[string]any{"type": "string", "minLength": 8}},
				"required":   []any{"token"},
			},
			"tokens": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string", "minLength": 8, "x-radius-sensitive": true},
			},
			"accounts": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type": "object", "x-radius-sensitive": true,
					"properties": map[string]any{"token": map[string]any{"type": "string", "minLength": 8}},
					"required":   []any{"token"},
				},
			},
		},
		"required": []any{"tls"},
	}

	for _, tt := range []struct {
		name       string
		properties map[string]any
		field      string
	}{
		{name: "valid enum", properties: map[string]any{"tls": "required"}},
		{name: "invalid enum", properties: map[string]any{"tls": "invalid"}, field: "tls"},
		{name: "wrong type", properties: map[string]any{"tls": 42}, field: "tls"},
		{name: "missing required", properties: map[string]any{}, field: "tls"},
		{name: "valid sensitive plaintext", properties: map[string]any{"tls": "required", "password": "secretvalue"}},
		{name: "sensitive length", properties: map[string]any{"tls": "required", "password": "short"}, field: "password"},
		{name: "sensitive pattern", properties: map[string]any{"tls": "required", "password": "SECRET123"}, field: "password"},
		{name: "encrypted object is not plaintext", properties: map[string]any{"tls": "required", "password": map[string]any{"encrypted": "secretvalue"}}, field: "password"},
		{name: "valid sensitive object", properties: map[string]any{"tls": "required", "credentials": map[string]any{"token": "secretvalue"}}},
		{name: "sensitive object required field", properties: map[string]any{"tls": "required", "credentials": map[string]any{"encrypted": "secretvalue"}}, field: "credentials"},
		{name: "sensitive object nested constraint", properties: map[string]any{"tls": "required", "credentials": map[string]any{"token": "short"}}, field: "credentials"},
		{name: "valid sensitive array", properties: map[string]any{"tls": "required", "tokens": []any{"secretvalue"}}},
		{name: "invalid sensitive array", properties: map[string]any{"tls": "required", "tokens": []any{"short"}}, field: "tokens"},
		{name: "valid sensitive map", properties: map[string]any{"tls": "required", "accounts": map[string]any{"alice": map[string]any{"token": "secretvalue"}}}},
		{name: "invalid sensitive map", properties: map[string]any{"tls": "required", "accounts": map[string]any{"private-account": map[string]any{"token": "short"}}}, field: "accounts"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			before, err := json.Marshal(schema)
			require.NoError(t, err)
			err = ValidateResourceRequestAgainstSchema(t.Context(), map[string]any{"properties": tt.properties}, schema)
			if tt.field == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.field)
				for _, sensitive := range []string{"secretvalue", "short", "SECRET123", "private-account"} {
					require.NotContains(t, err.Error(), sensitive)
				}
			}
			after, err := json.Marshal(schema)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
		})
	}
}

func TestValidateResourceRequestAgainstSchema_NullProperties(t *testing.T) {
	t.Parallel()
	for _, nullable := range []bool{false, true} {
		for _, tt := range []struct {
			name       string
			properties any
			isNull     bool
		}{
			{name: "null", properties: nil, isNull: true},
			{name: "nil datamodel map", properties: map[string]any(nil), isNull: true},
			{name: "empty object", properties: map[string]any{}},
		} {
			name := tt.name
			if nullable {
				name += " nullable"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				schema := map[string]any{
					"type": "object", "nullable": nullable,
					"properties": map[string]any{"name": map[string]any{"type": "string"}},
				}
				err := ValidateResourceRequestAgainstSchema(t.Context(), map[string]any{"properties": tt.properties}, schema)
				if tt.isNull && !nullable {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}

	t.Run("missing properties wrapper", func(t *testing.T) {
		err := ValidateResourceRequestAgainstSchema(t.Context(), map[string]any{}, map[string]any{"type": "object"})
		require.ErrorContains(t, err, "missing 'properties'")
	})
	t.Run("no schema", func(t *testing.T) {
		require.NoError(t, ValidateResourceRequestAgainstSchema(t.Context(), map[string]any{}, nil))
	})
}

func TestValidateResourceRequestAgainstSchema_RedactedSensitiveField(t *testing.T) {
	t.Parallel()
	for _, nullable := range []bool{false, true} {
		name := "non-nullable"
		if nullable {
			name = "nullable"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			schema := map[string]any{
				"type": "object",
				"properties": map[string]any{
					"password": map[string]any{"type": "string", "nullable": nullable, "x-radius-sensitive": true},
				},
			}
			err := ValidateResourceRequestAgainstSchema(t.Context(), map[string]any{"properties": map[string]any{"password": nil}}, schema)
			if nullable {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "password")
			}
		})
	}
}

func TestValidateResourceRequestAgainstSchema_CompositeSensitiveError(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"credentials": map[string]any{
				"type": "object", "x-radius-sensitive": true,
				"additionalProperties": map[string]any{
					"anyOf": []any{
						map[string]any{"type": "string", "enum": []any{"allowed"}},
						map[string]any{"type": "integer"},
					},
				},
			},
		},
	}
	err := ValidateResourceRequestAgainstSchema(t.Context(), map[string]any{
		"properties": map[string]any{"credentials": map[string]any{"private-key": "private-value"}},
	}, schema)
	require.ErrorContains(t, err, "credentials")
	require.NotContains(t, err.Error(), "private-key")
	require.NotContains(t, err.Error(), "private-value")
}

func TestFormatResourceRequestValidationError_UnknownError(t *testing.T) {
	t.Parallel()
	for _, err := range []error{
		errors.New("private-value"),
		openapi3.MultiError{errors.New("private-value")},
	} {
		result := formatResourceRequestValidationError(err, &openapi3.Schema{})
		require.EqualError(t, result, "resource data validation failed: value does not match the schema")
	}
}
