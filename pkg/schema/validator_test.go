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

package schema

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestNewValidator(t *testing.T) {
	validator := NewValidator()
	require.NotNil(t, validator)
	require.IsType(t, &Validator{}, validator)
}

func TestValidator_ValidateSchema(t *testing.T) {
	validator := NewValidator()
	ctx := context.Background()

	t.Run("nil schema", func(t *testing.T) {
		err := validator.ValidateSchema(ctx, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "schema cannot be nil")
	})

	t.Run("valid simple schema", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"string"},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})

	t.Run("valid object schema", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"name": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"age": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"integer"},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})
}

func TestValidator_checkProhibitedFeatures(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name   string
		schema *openapi3.Schema
		hasErr bool
		errMsg string
	}{
		{
			name: "allOf not allowed",
			schema: &openapi3.Schema{
				AllOf: []*openapi3.SchemaRef{
					{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
				},
			},
			hasErr: true,
			errMsg: "allOf is not supported",
		},
		{
			name: "anyOf not allowed",
			schema: &openapi3.Schema{
				AnyOf: []*openapi3.SchemaRef{
					{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
				},
			},
			hasErr: true,
			errMsg: "anyOf is not supported",
		},
		{
			name: "oneOf not allowed",
			schema: &openapi3.Schema{
				OneOf: []*openapi3.SchemaRef{
					{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
				},
			},
			hasErr: true,
			errMsg: "oneOf is not supported",
		},
		{
			name: "not not allowed",
			schema: &openapi3.Schema{
				Not: &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
			},
			hasErr: true,
			errMsg: "not is not supported",
		},
		{
			name: "discriminator not allowed",
			schema: &openapi3.Schema{
				Discriminator: &openapi3.Discriminator{
					PropertyName: "type",
				},
			},
			hasErr: true,
			errMsg: "discriminator is not supported",
		},
		{
			name: "valid schema without prohibited features",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
			},
			hasErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.checkProhibitedFeatures(tt.schema)
			if tt.hasErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
				// Check that it's a ConstraintError
				var constraintErr *ValidationError
				require.ErrorAs(t, err, &constraintErr)
				require.Equal(t, ErrorTypeConstraint, constraintErr.Type)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidator_validateTypeConstraints(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name   string
		schema *openapi3.Schema
		hasErr bool
		errMsg string
	}{
		{
			name: "string type allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
			},
			hasErr: false,
		},
		{
			name: "number type allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"number"},
			},
			hasErr: false,
		},
		{
			name: "integer type allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"integer"},
			},
			hasErr: false,
		},
		{
			name: "boolean type allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"boolean"},
			},
			hasErr: false,
		},
		{
			name: "object type allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
			},
			hasErr: false,
		},
		{
			name: "array type not allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"array"},
			},
			hasErr: true,
			errMsg: "unsupported type: array",
		},
		{
			name: "null type not allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"null"},
			},
			hasErr: true,
			errMsg: "unsupported type: null",
		},
		{
			name:   "no type specified (valid)",
			schema: &openapi3.Schema{
				// Type is nil - this should be valid
			},
			hasErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateTypeConstraints(tt.schema)
			if tt.hasErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)

				// Check that it's a ConstraintError
				var constraintErr *ValidationError
				require.ErrorAs(t, err, &constraintErr)
				require.Equal(t, ErrorTypeConstraint, constraintErr.Type)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConvertToOpenAPISchema(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid object schema",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid string schema",
			input: map[string]any{
				"type": "string",
			},
			wantErr: false,
		},
		{
			name:    "invalid schema data - function",
			input:   func() {},
			wantErr: true,
			errMsg:  "failed to marshal schema",
		},
		{
			name: "invalid JSON structure",
			input: map[string]any{
				"type": []func(){}, // Functions can't be marshaled
			},
			wantErr: true,
			errMsg:  "failed to marshal schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := ConvertToOpenAPISchema(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
				require.Nil(t, schema)
			} else {
				require.NoError(t, err)
				require.NotNil(t, schema)
			}
		})
	}
}

func TestValidator_validateRadiusConstraints_NestedProperties(t *testing.T) {
	validator := NewValidator()

	t.Run("nested object with valid properties", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"user": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"name": {
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"string"},
								},
							},
						},
					},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.NoError(t, err)
	})

	t.Run("nested object with invalid property", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"user": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"data": {
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"array"}, // Not allowed
								},
							},
						},
					},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "user.data")
		require.Contains(t, err.Error(), "unsupported type: array")
	})

	t.Run("additionalProperties validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Has: &[]bool{true}[0],
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.NoError(t, err)
	})

	t.Run("invalid additionalProperties", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Has: &[]bool{true}[0],
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"}, // Not allowed
					},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "additionalProperties")
		require.Contains(t, err.Error(), "unsupported type: array")
	})
}
