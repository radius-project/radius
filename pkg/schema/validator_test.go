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
	"fmt"
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
		require.NoError(t, err)
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
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})
}

func TestValidator_ValidateSchema_PlatformOptionsTypeAny(t *testing.T) {
	validator := NewValidator()
	ctx := context.Background()

	t.Run("platformOptions additionalProperties type any succeeds", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"platformOptions": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{
							Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"any"}}},
						},
					},
				},
			},
		}

		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})

	t.Run("non-platform additionalProperties type any fails", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"otherOptions": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{
							Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"any"}}},
						},
					},
				},
			},
		}

		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported 'type' value \"any\"")
	})
}

func TestNormalizePlatformOptionsAny(t *testing.T) {
	platformSchema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		AdditionalProperties: openapi3.AdditionalProperties{
			Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"any"}}},
		},
	}
	otherSchema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		AdditionalProperties: openapi3.AdditionalProperties{
			Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"any"}}},
		},
	}

	root := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: openapi3.Schemas{
			"platformOptions": {Value: platformSchema},
			"otherOptions":    {Value: otherSchema},
		},
	}

	normalizePlatformOptionsAny(root)

	platformAdditional := platformSchema.AdditionalProperties.Schema.Value
	otherAdditional := otherSchema.AdditionalProperties.Schema.Value

	require.NotNil(t, platformAdditional)
	require.Nil(t, platformAdditional.Type)
	require.NotNil(t, otherAdditional)
	require.NotNil(t, otherAdditional.Type)
	require.Equal(t, "any", (*otherAdditional.Type)[0])
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
			name: "array type allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"array"},
			},
			hasErr: false,
		},
		{
			name: "enum type allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"enum"},
			},
			hasErr: false,
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
			err := validator.validateTypeConstraints(tt.schema, "")
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
				"type": func() {}, // Functions can't be marshaled
			},
			wantErr: true,
			errMsg:  "failed to marshal schema",
		},
		{
			name: "valid complex schema with validation",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":      "string",
								"minLength": 1,
							},
							"email": map[string]any{
								"type":   "string",
								"format": "email",
							},
						},
					},
					"metadata": map[string]any{
						"type": "object",
						"additionalProperties": map[string]any{
							"type": "string",
						},
					},
				},
			},
			wantErr: false,
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
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
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
									Type: &openapi3.Types{"invalidtype"}, // Not allowed
								},
							},
						},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "user.data")
		require.Contains(t, err.Error(), "unsupported type: invalidtype")
	})

	t.Run("additionalProperties schema validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{
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

	t.Run("invalid additionalProperties schema", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"invalidtype"}, // Not allowed
					},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "additionalProperties")
		require.Contains(t, err.Error(), "unsupported type: invalidtype")
	})
}

func TestValidator_checkRefUsage(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name      string
		schemaRef *openapi3.SchemaRef
		fieldPath string
		hasErr    bool
		errMsg    string
	}{
		{
			name: "internal $ref in root schema - allowed",
			schemaRef: &openapi3.SchemaRef{
				Ref: "#/components/schemas/SomeSchema",
			},
			fieldPath: "",
			hasErr:    false,
		},
		{
			name: "external $ref with file path - not allowed",
			schemaRef: &openapi3.SchemaRef{
				Ref: "other-file.yaml#/components/schemas/SomeSchema",
			},
			fieldPath: "",
			hasErr:    true,
			errMsg:    "external $ref references are not supported",
		},
		{
			name: "external $ref with URL - not allowed",
			schemaRef: &openapi3.SchemaRef{
				Ref: "https://example.com/schema.json#/MySchema",
			},
			fieldPath: "",
			hasErr:    true,
			errMsg:    "external $ref references are not supported",
		},
		{
			name: "external $ref with relative path - not allowed",
			schemaRef: &openapi3.SchemaRef{
				Ref: "../schemas/common.yaml#/definitions/CommonType",
			},
			fieldPath: "",
			hasErr:    true,
			errMsg:    "external $ref references are not supported",
		},
		{
			name: "internal $ref in property - allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"prop1": {
							Ref: "#/components/schemas/SomeSchema",
						},
					},
				},
			},
			fieldPath: "",
			hasErr:    false,
		},
		{
			name: "external $ref in property - not allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"prop1": {
							Ref: "external.yaml#/components/schemas/SomeSchema",
						},
					},
				},
			},
			fieldPath: "",
			hasErr:    true,
			errMsg:    "external $ref references are not supported",
		},
		{
			name: "internal $ref in additionalProperties - allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					AdditionalProperties: openapi3.AdditionalProperties{
						Schema: &openapi3.SchemaRef{
							Ref: "#/components/schemas/SomeSchema",
						},
					},
				},
			},
			fieldPath: "",
			hasErr:    false,
		},
		{
			name: "external $ref in additionalProperties - not allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					AdditionalProperties: openapi3.AdditionalProperties{
						Schema: &openapi3.SchemaRef{
							Ref: "external.yaml#/components/schemas/SomeSchema",
						},
					},
				},
			},
			fieldPath: "",
			hasErr:    true,
			errMsg:    "external $ref references are not supported",
		},
		{
			name: "internal $ref in array items - allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"array"},
					Items: &openapi3.SchemaRef{
						Ref: "#/components/schemas/SomeSchema",
					},
				},
			},
			fieldPath: "",
			hasErr:    false,
		},
		{
			name: "external $ref in array items - not allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"array"},
					Items: &openapi3.SchemaRef{
						Ref: "external.yaml#/components/schemas/SomeSchema",
					},
				},
			},
			fieldPath: "",
			hasErr:    true,
			errMsg:    "external $ref references are not supported",
		},
		{
			name: "external $ref in nested property - not allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"parent": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: map[string]*openapi3.SchemaRef{
									"child": {
										Ref: "external.yaml#/components/schemas/SomeSchema",
									},
								},
							},
						},
					},
				},
			},
			fieldPath: "",
			hasErr:    true,
			errMsg:    "external $ref references are not supported",
		},
		{
			name: "internal $ref in allOf - allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					AllOf: []*openapi3.SchemaRef{
						{
							Ref: "#/components/schemas/SomeSchema",
						},
					},
				},
			},
			fieldPath: "",
			hasErr:    false,
		},
		{
			name: "external $ref in allOf - not allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					AllOf: []*openapi3.SchemaRef{
						{
							Ref: "external.yaml#/components/schemas/SomeSchema",
						},
					},
				},
			},
			fieldPath: "",
			hasErr:    true,
			errMsg:    "external $ref references are not supported",
		},
		{
			name: "internal $ref in not - allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Not: &openapi3.SchemaRef{
						Ref: "#/components/schemas/SomeSchema",
					},
				},
			},
			fieldPath: "",
			hasErr:    false,
		},
		{
			name: "external $ref in not - not allowed",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Not: &openapi3.SchemaRef{
						Ref: "external.yaml#/components/schemas/SomeSchema",
					},
				},
			},
			fieldPath: "",
			hasErr:    true,
			errMsg:    "external $ref references are not supported",
		},
		{
			name: "valid schema without $ref",
			schemaRef: &openapi3.SchemaRef{
				Value: &openapi3.Schema{
					Type: &openapi3.Types{"object"},
					Properties: map[string]*openapi3.SchemaRef{
						"prop1": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"string"},
							},
						},
						"prop2": {
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"integer"},
							},
						},
					},
				},
			},
			fieldPath: "",
			hasErr:    false,
		},
		{
			name:      "nil schema ref",
			schemaRef: nil,
			fieldPath: "",
			hasErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.checkRefUsage(tt.schemaRef, tt.fieldPath)
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

// Test that $ref validation is integrated into the main validation flow
func TestValidator_RefValidationIntegration(t *testing.T) {
	validator := NewValidator()

	t.Run("schema with internal $ref in property passes validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: map[string]*openapi3.SchemaRef{
				"goodProp": {
					Ref: "#/components/schemas/SomeSchema",
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.NoError(t, err)
	})

	t.Run("schema with external $ref in property fails validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: map[string]*openapi3.SchemaRef{
				"badProp": {
					Ref: "external.yaml#/components/schemas/SomeSchema",
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "external $ref references are not supported")
	})

	t.Run("schema with internal $ref in additionalProperties passes validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Schema: &openapi3.SchemaRef{
					Ref: "#/components/schemas/SomeSchema",
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.NoError(t, err)
	})

	t.Run("schema with external $ref in additionalProperties fails validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Schema: &openapi3.SchemaRef{
					Ref: "external.yaml#/components/schemas/SomeSchema",
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "external $ref references are not supported")
	})
}

func TestValidator_isInternalRef(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{
			name:     "empty string",
			ref:      "",
			expected: false,
		},
		{
			name:     "internal ref - components/schemas",
			ref:      "#/components/schemas/MySchema",
			expected: true,
		},
		{
			name:     "internal ref - definitions",
			ref:      "#/definitions/SomeType",
			expected: true,
		},
		{
			name:     "internal ref - root fragment",
			ref:      "#/",
			expected: true,
		},
		{
			name:     "internal ref - just hash",
			ref:      "#",
			expected: true,
		},
		{
			name:     "external ref - relative path",
			ref:      "other-file.yaml#/components/schemas/MySchema",
			expected: false,
		},
		{
			name:     "external ref - absolute path",
			ref:      "/absolute/path/schema.yaml#/definitions/Type",
			expected: false,
		},
		{
			name:     "external ref - URL",
			ref:      "https://example.com/schema.json#/MySchema",
			expected: false,
		},
		{
			name:     "external ref - relative directory",
			ref:      "../schemas/common.yaml#/definitions/CommonType",
			expected: false,
		},
		{
			name:     "external ref - no fragment",
			ref:      "external.yaml",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.isInternalRef(tt.ref)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_checkObjectPropertyConstraints(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name       string
		schema     *openapi3.Schema
		path       string
		hasErr     bool
		errMsg     string
		expectPath bool
	}{
		{
			name: "object with only properties - allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"name": {
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
					"age": {
						Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}},
					},
				},
			},
			hasErr: false,
			path:   "spec",
		},
		{
			name: "object with only additionalProperties schema - allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
			},
			path:   "spec",
			hasErr: false,
		},
		{
			name: "object with both properties and additionalProperties schema - not allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"name": {
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
			},
			path:   "spec",
			hasErr: true,
			errMsg: "object schemas cannot have both 'properties' and 'additionalProperties' defined",
		},
		{
			name: "object with additionalProperties set to false - allowed with properties",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"name": {
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
				AdditionalProperties: openapi3.AdditionalProperties{
					Has: &[]bool{false}[0],
				},
			},
			path:   "spec",
			hasErr: false,
		},
		{
			name: "object with empty properties and additionalProperties schema - allowed",
			schema: &openapi3.Schema{
				Type:       &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{}, // Empty properties map
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
			},
			path:   "spec",
			hasErr: false,
		},
		{
			name: "non-object schema with both properties and additionalProperties - allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"}, // Not an object type
				Properties: map[string]*openapi3.SchemaRef{
					"name": {
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
				AdditionalProperties: openapi3.AdditionalProperties{
					Has: &[]bool{true}[0],
				},
			},
			path:   "spec",
			hasErr: false, // Constraint only applies to object types
		},
		{
			name: "typeless schema with both properties and additionalProperties schema - not allowed",
			schema: &openapi3.Schema{
				// No type specified, but has object-like properties
				Properties: map[string]*openapi3.SchemaRef{
					"name": {
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
			},
			path:   "spec",
			hasErr: true,
			errMsg: "object schemas cannot have both 'properties' and 'additionalProperties' defined",
		},
		{
			name: "object with no properties or additionalProperties - allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
			},
			path:   "spec",
			hasErr: false,
		},
		{
			name: "object with additionalProperties: true - not allowed",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Has: &[]bool{true}[0],
				},
			},
			path:   "spec",
			hasErr: true,
			errMsg: "additionalProperties: true is not allowed, use a schema object instead",
		},
		{
			name: "typeless schema with additionalProperties: true - not allowed",
			schema: &openapi3.Schema{
				Properties: map[string]*openapi3.SchemaRef{
					"name": {
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
				AdditionalProperties: openapi3.AdditionalProperties{
					Has: &[]bool{true}[0],
				},
			},
			path:   "spec",
			hasErr: true,
			errMsg: "additionalProperties: true is not allowed, use a schema object instead",
		},
		{
			name: "platformOptions allows unconstrained additionalProperties",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{}},
				},
			},
			path:   "spec.platformOptions",
			hasErr: false,
		},
		{
			name: "non-platform property rejects unconstrained additionalProperties",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{}},
				},
			},
			path:       "spec.otherOptions",
			hasErr:     true,
			errMsg:     "additionalProperties may be type `any` only for the platformOptions property",
			expectPath: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "non-platform property rejects unconstrained additionalProperties" {
				require.NotNil(t, tt.schema.AdditionalProperties.Schema)
				require.NotNil(t, tt.schema.AdditionalProperties.Schema.Value)
				require.True(t, isUnconstrainedSchema(tt.schema.AdditionalProperties.Schema.Value))
			}
			err := validator.checkObjectPropertyConstraints(tt.schema, tt.path)
			if tt.hasErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
				// Check that it's a ConstraintError
				var constraintErr *ValidationError
				require.ErrorAs(t, err, &constraintErr)
				require.Equal(t, ErrorTypeConstraint, constraintErr.Type)
				if tt.expectPath {
					require.Contains(t, err.Error(), tt.path)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test integration of object property constraints with main validation
func TestValidator_ObjectPropertyConstraintsIntegration(t *testing.T) {
	validator := NewValidator()

	t.Run("schema with both properties and additionalProperties schema fails main validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: map[string]*openapi3.SchemaRef{
				"name": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
			},
			AdditionalProperties: openapi3.AdditionalProperties{
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "object schemas cannot have both 'properties' and 'additionalProperties' defined")
	})

	t.Run("schema with only properties passes main validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: map[string]*openapi3.SchemaRef{
				"name": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"environment": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.NoError(t, err)
	})

	t.Run("schema with only additionalProperties schema passes main validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.NoError(t, err)
	})

	t.Run("schema with additionalProperties: true fails main validation", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Has: &[]bool{true}[0],
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "additionalProperties: true is not allowed, use a schema object instead")
	})

	t.Run("schema allows platformOptions additionalProperties any", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: map[string]*openapi3.SchemaRef{
				"platformOptions": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{
							Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{}},
						},
					},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.NoError(t, err)
	})

	t.Run("schema rejects non-platform any additionalProperties", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: map[string]*openapi3.SchemaRef{
				"otherOptions": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{
							Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{}},
						},
					},
				},
			},
		}
		err := validator.validateRadiusConstraints(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "additionalProperties may be type `any` only for the platformOptions property")
	})
}

func TestValidator_validateSchemaWithOpenAPI(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		schema  *openapi3.Schema
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid simple string schema",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
			},
			wantErr: false,
		},
		{
			name: "valid object schema with properties",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"name": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"string"},
						},
					},
					"age": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"integer"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid schema with format constraints",
			schema: &openapi3.Schema{
				Type:   &openapi3.Types{"string"},
				Format: "email",
			},
			wantErr: false,
		},
		{
			name: "valid schema with pattern",
			schema: &openapi3.Schema{
				Type:    &openapi3.Types{"string"},
				Pattern: "^[a-z]+$",
			},
			wantErr: false,
		},
		{
			name: "valid number schema with range constraints",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"number"},
				Min:  &[]float64{0}[0],
				Max:  &[]float64{100}[0],
			},
			wantErr: false,
		},
		{
			name: "valid integer schema with range constraints",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"integer"},
				Min:  &[]float64{1}[0],
				Max:  &[]float64{1000}[0],
			},
			wantErr: false,
		},
		{
			name: "valid string schema with length constraints",
			schema: &openapi3.Schema{
				Type:      &openapi3.Types{"string"},
				MinLength: 1,
				MaxLength: &[]uint64{255}[0],
			},
			wantErr: false,
		},
		{
			name: "valid schema with enum values",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
				Enum: []any{"red", "green", "blue"},
			},
			wantErr: false,
		},
		{
			name: "valid boolean schema",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"boolean"},
			},
			wantErr: false,
		},
		{
			name: "valid schema without internal reference dependency",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"name": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"string"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid array schema with items",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"array"},
				Items: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid schema with additionalProperties",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				AdditionalProperties: openapi3.AdditionalProperties{
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"string"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid typeless schema",
			schema: &openapi3.Schema{
				Description: "A schema without explicit type",
			},
			wantErr: false,
		},
		{
			name: "schema with invalid regex pattern",
			schema: &openapi3.Schema{
				Type:    &openapi3.Types{"string"},
				Pattern: "[invalid regex",
			},
			wantErr: true,
			errMsg:  "OpenAPI document validation failed",
		},
		{
			name: "schema with complex nested structure",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"address": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"object"},
							Properties: map[string]*openapi3.SchemaRef{
								"street": {
									Value: &openapi3.Schema{
										Type:      &openapi3.Types{"string"},
										MinLength: 1,
									},
								},
								"zipCode": {
									Value: &openapi3.Schema{
										Type:    &openapi3.Types{"string"},
										Pattern: "^\\d{5}$",
									},
								},
							},
						},
					},
					"contacts": {
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"array"},
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"object"},
									Properties: map[string]*openapi3.SchemaRef{
										"email": {
											Value: &openapi3.Schema{
												Type:   &openapi3.Types{"string"},
												Format: "email",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "schema with invalid nested pattern",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: map[string]*openapi3.SchemaRef{
					"invalidField": {
						Value: &openapi3.Schema{
							Type:    &openapi3.Types{"string"},
							Pattern: "[unclosed bracket",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "OpenAPI document validation failed",
		},
		{
			name:    "nil schema",
			schema:  nil,
			wantErr: true,
			errMsg:  "OpenAPI document validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateSchemaWithOpenAPI(tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateSchema_EdgeCases(t *testing.T) {
	validator := NewValidator()
	ctx := context.Background()

	t.Run("schema with empty object", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
		}

		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})

	t.Run("schema with additionalProperties: true should fail", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			AdditionalProperties: openapi3.AdditionalProperties{
				Has: &[]bool{true}[0],
			},
		}

		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "additionalProperties: true is not allowed, use a schema object instead")
	})

	t.Run("schema with multiple types should fail", func(t *testing.T) {
		// OpenAPI 3.0 doesn't support multiple types in the same way as JSON Schema
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"string", "null"}, // Multiple types
		}

		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "OpenAPI document validation failed")
	})

	t.Run("schema with unknown format should be valid", func(t *testing.T) {
		// OpenAPI should allow unknown formats (they are just hints)
		schema := &openapi3.Schema{
			Type:   &openapi3.Types{"string"},
			Format: "custom-format",
		}

		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})
}

func TestValidator_checkReservedProperties(t *testing.T) {
	validator := NewValidator()

	t.Run("nil properties", func(t *testing.T) {
		schema := &openapi3.Schema{}
		err := validator.checkReservedProperties(schema)
		require.NoError(t, err)
	})

	t.Run("valid properties", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"validProp": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.NoError(t, err)
	})

	t.Run("status property is not allowed", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"status": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 1)
		require.Equal(t, "status", validationErrors.Errors[0].Field)
		require.Contains(t, validationErrors.Errors[0].Message, "property 'status' is reserved and cannot be used")
	})

	t.Run("recipe property is not allowed", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"recipe": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 2) // recipe + environment
		// Check that recipe error is present
		errorFields := make(map[string]bool)
		for _, ve := range validationErrors.Errors {
			errorFields[ve.Field] = true
		}
		require.Contains(t, errorFields, "recipe")
		require.Contains(t, errorFields, "environment")
		require.Contains(t, validationErrors.Errors[0].Message, "property 'recipe' is reserved and cannot be used")
	})

	t.Run("application property must be string", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"application": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"integer"},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 1)
		require.Equal(t, "application", validationErrors.Errors[0].Field)
		require.Contains(t, validationErrors.Errors[0].Message, "property 'application' must be a string")
	})

	t.Run("valid application property as string", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"application": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.NoError(t, err)
	})

	t.Run("environment property must be string", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"number"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 1)
		require.Equal(t, "environment", validationErrors.Errors[0].Field)
		require.Contains(t, validationErrors.Errors[0].Message, "property 'environment' must be a string")
	})

	t.Run("valid environment property as string", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.NoError(t, err)
	})

	t.Run("connections property must be object", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"connections": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 1)
		require.Equal(t, "connections", validationErrors.Errors[0].Field)
		require.Contains(t, validationErrors.Errors[0].Message, "property 'connections' must be a map object")
	})

	t.Run("valid connections property as map with additionalProperties schema", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"connections": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
							},
						},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.NoError(t, err)
	})

	t.Run("connections property without additionalProperties should fail", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"connections": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						// No additionalProperties - should fail
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 1)
		require.Equal(t, "connections", validationErrors.Errors[0].Field)
		require.Contains(t, validationErrors.Errors[0].Message, "must be a map object (use additionalProperties)")
	})

	t.Run("connections property with additionalProperties: true should fail", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"connections": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{
							Has: &[]bool{true}[0], // This should fail
						},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.NoError(t, err) // checkReservedProperties doesn't enforce the additionalProperties: true restriction
	})

	t.Run("connections property with fixed properties should fail", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"connections": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"fixedProp": &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"string"},
								},
							},
						},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 1)
		require.Equal(t, "connections", validationErrors.Errors[0].Field)
		require.Contains(t, validationErrors.Errors[0].Message, "must be a map object (use additionalProperties)")
	})

	t.Run("environment property always required", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"application": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
			// environment missing from properties - should always fail, regardless of Required array
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 1)
		require.Equal(t, "environment", validationErrors.Errors[0].Field)
		require.Contains(t, validationErrors.Errors[0].Message, "property 'environment' must be included in schema")
	})

	t.Run("environment property missing from any schema should fail", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"someOtherProp": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
			// No Required array, no environment property - should still fail
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 1)
		require.Equal(t, "environment", validationErrors.Errors[0].Field)
		require.Contains(t, validationErrors.Errors[0].Message, "property 'environment' must be included in schema")
	})

	t.Run("environment property present", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
			Required: []string{"environment"},
		}
		err := validator.checkReservedProperties(schema)
		require.NoError(t, err)
	})

	t.Run("property with nil value", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"application": &openapi3.SchemaRef{
					Value: nil,
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.NoError(t, err)
	})

	t.Run("property with nil type", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"application": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: nil,
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "property 'application' must be a string")
	})

	t.Run("multiple violations", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"status": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"application": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"integer"},
					},
				},
			},
		}
		err := validator.checkReservedProperties(schema)
		require.Error(t, err)
		// Should now collect all violations
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 3) // status, application, environment

		// Check that all errors are present
		errorFields := make(map[string]bool)
		for _, ve := range validationErrors.Errors {
			errorFields[ve.Field] = true
		}
		require.Contains(t, errorFields, "status")
		require.Contains(t, errorFields, "application")
		require.Contains(t, errorFields, "environment")
	})
}

func TestValidator_ValidateSchema_MultipleErrors(t *testing.T) {
	validator := NewValidator()
	ctx := context.Background()

	t.Run("always collects all errors", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"status": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"application": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"integer"},
					},
				},
				"connections": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"boolean"},
					},
				},
			},
		}

		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)

		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.GreaterOrEqual(t, len(validationErrors.Errors), 4) // At least status, application, connections, environment

		t.Logf("Collected errors:\n%s", err.Error())
	})

	t.Run("single error still returns ValidationErrors", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"status": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"environment": &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}

		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)

		// Should still be a ValidationErrors collection, not a single ValidationError
		validationErrors, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.Len(t, validationErrors.Errors, 1)
		require.Equal(t, "status", validationErrors.Errors[0].Field)
	})
}

func TestValidateResourceAgainstSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("nil schema returns nil", func(t *testing.T) {
		resourceData := map[string]any{
			"properties": map[string]any{
				"name": "test",
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, nil)
		require.NoError(t, err)
	})

	t.Run("missing properties field", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type": "string",
				},
			},
		}

		resourceData := map[string]any{
			"name": "test", // missing properties wrapper
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resource data missing 'properties' field")
	})

	t.Run("valid resource against object schema", func(t *testing.T) {
		// Object schema with properties
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type": "string",
				},
				"count": map[string]any{
					"type": "integer",
				},
			},
			"required": []any{"name"},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"name":  "test-resource",
				"count": 42,
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

	t.Run("invalid resource against schema - missing required field", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type": "string",
				},
			},
			"required": []any{"name"},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"count": 42, // missing required "name" field
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resource data validation failed")
		require.Contains(t, err.Error(), "name")
	})

	t.Run("invalid resource against schema - wrong type", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{
					"type": "integer",
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"count": "not a number", // should be integer
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resource data validation failed")
		require.Contains(t, err.Error(), "count")
	})

	t.Run("invalid schema format", func(t *testing.T) {
		// Invalid schema that can't be converted
		schema := "invalid schema format"

		resourceData := map[string]any{
			"properties": map[string]any{
				"name": "test",
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to convert schema")
	})

	t.Run("empty properties data", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
		}

		resourceData := map[string]any{
			"properties": map[string]any{},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err) // empty object is valid against object schema
	})

	t.Run("platformOptions additionalProperties type any", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"platformOptions": map[string]any{
					"type": "object",
					"additionalProperties": map[string]any{
						"type": "any",
					},
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"platformOptions": map[string]any{
					"custom": map[string]any{
						"flag": true,
					},
				},
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

	t.Run("structured error message with field path", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"user": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"age": map[string]any{
							"type": "integer",
						},
					},
					"required": []any{"age"},
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"user": map[string]any{
					"name": "john", // missing required age field
				},
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "property \"age\" is missing")
	})
}

func TestValidator_checkSensitiveAnnotation(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name   string
		schema *openapi3.Schema
		path   string
		hasErr bool
		errMsg string
	}{
		{
			name: "x-radius-sensitive on string type - valid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
				Extensions: map[string]any{
					annotationRadiusSensitive: true,
				},
			},
			path:   "password",
			hasErr: false,
		},
		{
			name: "x-radius-sensitive on object type - valid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Extensions: map[string]any{
					annotationRadiusSensitive: true,
				},
			},
			path:   "credentials",
			hasErr: false,
		},
		{
			name: "x-radius-sensitive on integer type - invalid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"integer"},
				Extensions: map[string]any{
					annotationRadiusSensitive: true,
				},
			},
			path:   "count",
			hasErr: true,
			errMsg: fmt.Sprintf("%s annotation is only supported on string and object types, got 'integer'", annotationRadiusSensitive),
		},
		{
			name: "x-radius-sensitive on boolean type - invalid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"boolean"},
				Extensions: map[string]any{
					annotationRadiusSensitive: true,
				},
			},
			path:   "flag",
			hasErr: true,
			errMsg: fmt.Sprintf("%s annotation is only supported on string and object types, got 'boolean'", annotationRadiusSensitive),
		},
		{
			name: "x-radius-sensitive on array type - invalid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"array"},
				Extensions: map[string]any{
					annotationRadiusSensitive: true,
				},
			},
			path:   "items",
			hasErr: true,
			errMsg: fmt.Sprintf("%s annotation is only supported on string and object types, got 'array'", annotationRadiusSensitive),
		},
		{
			name: "x-radius-sensitive on number type - invalid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"number"},
				Extensions: map[string]any{
					annotationRadiusSensitive: true,
				},
			},
			path:   "value",
			hasErr: true,
			errMsg: fmt.Sprintf("%s annotation is only supported on string and object types, got 'number'", annotationRadiusSensitive),
		},
		{
			name: "x-radius-sensitive set to false - valid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"integer"},
				Extensions: map[string]any{
					annotationRadiusSensitive: false,
				},
			},
			path:   "count",
			hasErr: false,
		},
		{
			name: "no x-radius-sensitive annotation - valid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"integer"},
			},
			path:   "count",
			hasErr: false,
		},
		{
			name: "x-radius-sensitive with no type - invalid",
			schema: &openapi3.Schema{
				Extensions: map[string]any{
					annotationRadiusSensitive: true,
				},
			},
			path:   "field",
			hasErr: true,
			errMsg: fmt.Sprintf("%s annotation requires an explicit type (string or object)", annotationRadiusSensitive),
		},
		{
			name: "x-radius-sensitive with string value - invalid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
				Extensions: map[string]any{
					annotationRadiusSensitive: "true",
				},
			},
			path:   "password",
			hasErr: true,
			errMsg: fmt.Sprintf("%s must be a boolean value", annotationRadiusSensitive),
		},
		{
			name: "x-radius-sensitive with integer value - invalid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
				Extensions: map[string]any{
					annotationRadiusSensitive: 1,
				},
			},
			path:   "apiKey",
			hasErr: true,
			errMsg: fmt.Sprintf("%s must be a boolean value", annotationRadiusSensitive),
		},
		{
			name: "x-radius-sensitive with null value - invalid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
				Extensions: map[string]any{
					annotationRadiusSensitive: nil,
				},
			},
			path:   "token",
			hasErr: true,
			errMsg: fmt.Sprintf("%s must be a boolean value", annotationRadiusSensitive),
		},
		{
			name: "x-radius-sensitive on string enum - valid",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
				Enum: []any{"value1", "value2", "value3"},
				Extensions: map[string]any{
					annotationRadiusSensitive: true,
				},
			},
			path:   "status",
			hasErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.checkSensitiveAnnotation(tt.schema, tt.path)
			if tt.hasErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
				var constraintErr *ValidationError
				require.ErrorAs(t, err, &constraintErr)
				require.Equal(t, ErrorTypeConstraint, constraintErr.Type)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateSchema_WithSensitiveAnnotation(t *testing.T) {
	validator := NewValidator()
	ctx := context.Background()

	t.Run("valid string with x-radius-sensitive", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"password": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})

	t.Run("valid object with x-radius-sensitive", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"credentials": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
						},
						AdditionalProperties: openapi3.AdditionalProperties{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
							},
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})

	t.Run("invalid integer with x-radius-sensitive", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"count": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"integer"},
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("%s annotation is only supported on string and object types, got 'integer'", annotationRadiusSensitive))
	})

	t.Run("invalid nested property with x-radius-sensitive on boolean", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"config": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"enabled": {
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"boolean"},
									Extensions: map[string]any{
										annotationRadiusSensitive: true,
									},
								},
							},
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("%s annotation is only supported on string and object types, got 'boolean'", annotationRadiusSensitive))
	})

	t.Run("invalid array with x-radius-sensitive", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"tags": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
						},
						Items: &openapi3.SchemaRef{
							Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("%s annotation is only supported on string and object types, got 'array'", annotationRadiusSensitive))
	})

	t.Run("valid multiple sensitive strings in nested structure", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"auth": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"username": {
								Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
							},
							"password": {
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"string"},
									Extensions: map[string]any{
										annotationRadiusSensitive: true,
									},
								},
							},
							"apiKey": {
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"string"},
									Extensions: map[string]any{
										annotationRadiusSensitive: true,
									},
								},
							},
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})

	t.Run("invalid non-boolean x-radius-sensitive value", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"password": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
						Extensions: map[string]any{
							annotationRadiusSensitive: "true",
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("%s must be a boolean value", annotationRadiusSensitive))
	})

	t.Run("valid string enum with x-radius-sensitive", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"secretType": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
						Enum: []any{"apiKey", "password", "certificate"},
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})

	t.Run("valid array with sensitive string items", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"apiKeys": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"string"},
								Extensions: map[string]any{
									annotationRadiusSensitive: true,
								},
							},
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})

	t.Run("valid array with sensitive object items", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"secrets": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: openapi3.Schemas{
									"name": {
										Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
									},
									"token": {
										Value: &openapi3.Schema{
											Type: &openapi3.Types{"string"},
											Extensions: map[string]any{
												annotationRadiusSensitive: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.NoError(t, err)
	})

	t.Run("invalid array with sensitive integer items", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"environment": {
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
				"codes": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"integer"},
								Extensions: map[string]any{
									annotationRadiusSensitive: true,
								},
							},
						},
					},
				},
			},
		}
		err := validator.ValidateSchema(ctx, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("%s annotation is only supported on string and object types, got 'integer'", annotationRadiusSensitive))
	})
}

// requireSensitiveAnyOf asserts that a schema was normalized to an AnyOf[string, non-empty object]
// and returns the string branch for further inspection.
func requireSensitiveAnyOf(t *testing.T, s *openapi3.Schema) *openapi3.Schema {
	t.Helper()
	require.Nil(t, s.Type, "normalized schema should have no top-level type")
	require.Len(t, s.AnyOf, 2, "normalized schema should have two AnyOf branches")
	require.True(t, s.AnyOf[0].Value.Type.Is("string"), "first AnyOf branch should be string")
	require.True(t, s.AnyOf[1].Value.Type.Is("object"), "second AnyOf branch should be object")
	require.Equal(t, uint64(1), s.AnyOf[1].Value.MinProps, "object branch should require at least one property")
	return s.AnyOf[0].Value
}

func TestNormalizeSensitiveFieldTypes(t *testing.T) {
	t.Run("nil schema", func(t *testing.T) {
		// Should not panic
		normalizeSensitiveFieldTypes(nil)
	})

	t.Run("sensitive string field is normalized to AnyOf", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"password": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
						},
					},
				},
				"name": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		// Sensitive string field should be normalized to AnyOf[string, object]
		requireSensitiveAnyOf(t, schema.Properties["password"].Value)
		// Non-sensitive field should retain its type
		require.NotNil(t, schema.Properties["name"].Value.Type)
		require.True(t, schema.Properties["name"].Value.Type.Is("string"))
	})

	t.Run("sensitive object field retains type", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"credentials": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		// Sensitive object field should retain its type (objects stay objects after encryption)
		require.NotNil(t, schema.Properties["credentials"].Value.Type)
		require.True(t, schema.Properties["credentials"].Value.Type.Is("object"))
	})

	t.Run("nested sensitive string field is normalized to AnyOf", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"auth": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"username": {
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"string"},
								},
							},
							"secret": {
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"string"},
									Extensions: map[string]any{
										annotationRadiusSensitive: true,
									},
								},
							},
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		// Nested sensitive string should be normalized
		requireSensitiveAnyOf(t, schema.Properties["auth"].Value.Properties["secret"].Value)
		// Non-sensitive sibling should retain type
		require.NotNil(t, schema.Properties["auth"].Value.Properties["username"].Value.Type)
	})

	t.Run("sensitive string property inside array items", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"secrets": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"object"},
								Properties: openapi3.Schemas{
									"value": {
										Value: &openapi3.Schema{
											Type: &openapi3.Types{"string"},
											Extensions: map[string]any{
												annotationRadiusSensitive: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		// Sensitive string inside array items should be normalized
		requireSensitiveAnyOf(t, schema.Properties["secrets"].Value.Items.Value.Properties["value"].Value)
	})

	t.Run("sensitive string directly on array items schema", func(t *testing.T) {
		// e.g. an array of sensitive strings: items itself is marked sensitive
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"tokens": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"array"},
						Items: &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Type: &openapi3.Types{"string"},
								Extensions: map[string]any{
									annotationRadiusSensitive: true,
								},
							},
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		// The items schema itself should be normalized
		requireSensitiveAnyOf(t, schema.Properties["tokens"].Value.Items.Value)
	})

	t.Run("sensitive string property inside additionalProperties", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"config": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"object"},
									Properties: openapi3.Schemas{
										"token": {
											Value: &openapi3.Schema{
												Type: &openapi3.Types{"string"},
												Extensions: map[string]any{
													annotationRadiusSensitive: true,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		// Sensitive string inside additionalProperties should be normalized
		requireSensitiveAnyOf(t, schema.Properties["config"].Value.AdditionalProperties.Schema.Value.Properties["token"].Value)
	})

	t.Run("sensitive string directly on additionalProperties schema", func(t *testing.T) {
		// e.g. a map where all values are sensitive strings
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"secretMap": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						AdditionalProperties: openapi3.AdditionalProperties{
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"string"},
									Extensions: map[string]any{
										annotationRadiusSensitive: true,
									},
								},
							},
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		// The additionalProperties schema itself should be normalized
		requireSensitiveAnyOf(t, schema.Properties["secretMap"].Value.AdditionalProperties.Schema.Value)
	})

	t.Run("non-sensitive fields are not modified", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"name": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
				"count": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"integer"},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		require.True(t, schema.Properties["name"].Value.Type.Is("string"))
		require.True(t, schema.Properties["count"].Value.Type.Is("integer"))
	})

	t.Run("x-radius-sensitive false does not normalize type", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"token": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
						Extensions: map[string]any{
							annotationRadiusSensitive: false,
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		// x-radius-sensitive: false should not affect the type
		require.NotNil(t, schema.Properties["token"].Value.Type)
		require.True(t, schema.Properties["token"].Value.Type.Is("string"))
	})

	t.Run("sensitive string with additional constraints preserves them on string branch", func(t *testing.T) {
		maxLen := uint64(128)
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"apiKey": {
					Value: &openapi3.Schema{
						Type:      &openapi3.Types{"string"},
						MinLength: 8,
						MaxLength: &maxLen,
						Pattern:   "^[A-Za-z0-9]+$",
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		// The string branch of the AnyOf should preserve the original constraints
		// so that plaintext values are still validated.
		stringBranch := requireSensitiveAnyOf(t, schema.Properties["apiKey"].Value)
		require.Equal(t, uint64(8), stringBranch.MinLength)
		require.Equal(t, &maxLen, stringBranch.MaxLength)
		require.Equal(t, "^[A-Za-z0-9]+$", stringBranch.Pattern)
	})

	t.Run("preserves other vendor extensions on string branch", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"secret": {
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
							"x-custom-metadata":      "some-value",
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		stringBranch := requireSensitiveAnyOf(t, schema.Properties["secret"].Value)
		// x-radius-sensitive should be removed from the string branch
		_, hasSensitive := stringBranch.Extensions[annotationRadiusSensitive]
		require.False(t, hasSensitive, "x-radius-sensitive should be removed from string branch")
		// Other vendor extensions should be preserved
		require.Equal(t, "some-value", stringBranch.Extensions["x-custom-metadata"])
	})

	t.Run("nullable sensitive string preserves nullable on string branch", func(t *testing.T) {
		schema := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"token": {
					Value: &openapi3.Schema{
						Type:     &openapi3.Types{"string"},
						Nullable: true,
						Extensions: map[string]any{
							annotationRadiusSensitive: true,
						},
					},
				},
			},
		}

		normalizeSensitiveFieldTypes(schema)

		stringBranch := requireSensitiveAnyOf(t, schema.Properties["token"].Value)
		require.True(t, stringBranch.Nullable, "nullable flag should be preserved on string branch")
	})
}

func TestValidateResourceAgainstSchema_EncryptedSensitiveFields(t *testing.T) {
	ctx := context.Background()

	t.Run("encrypted sensitive string field passes validation", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type": "string",
				},
				"password": map[string]any{
					"type": "string",
					"x-radius-sensitive": true,
				},
			},
		}

		// After encryption, the string field becomes an object
		resourceData := map[string]any{
			"properties": map[string]any{
				"name": "my-resource",
				"password": map[string]any{
					"encrypted": "base64-ciphertext",
					"nonce":     "base64-nonce",
					"ad":        "base64-hash",
					"version":   1,
				},
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

	t.Run("unencrypted sensitive string field still passes validation", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"password": map[string]any{
					"type": "string",
					"x-radius-sensitive": true,
				},
			},
		}

		// Before encryption, the value is still a plain string
		resourceData := map[string]any{
			"properties": map[string]any{
				"password": "plaintext-password",
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

	t.Run("encrypted nested sensitive string field passes validation", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"auth": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"username": map[string]any{
							"type": "string",
						},
						"secret": map[string]any{
							"type": "string",
							"x-radius-sensitive": true,
						},
					},
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"auth": map[string]any{
					"username": "admin",
					"secret": map[string]any{
						"encrypted": "base64-ciphertext",
						"nonce":     "base64-nonce",
						"ad":        "base64-hash",
						"version":   1,
					},
				},
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

	t.Run("sensitive object field still passes validation when encrypted", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"credentials": map[string]any{
					"type": "object",
					"x-radius-sensitive": true,
				},
			},
		}

		// Sensitive object stays as an object after encryption but contents change
		resourceData := map[string]any{
			"properties": map[string]any{
				"credentials": map[string]any{
					"encrypted": "base64-ciphertext",
					"nonce":     "base64-nonce",
					"ad":        "base64-hash",
					"version":   1,
				},
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

	t.Run("encrypted sensitive string with constraints passes validation", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"apiKey": map[string]any{
					"type":      "string",
					"minLength": 8,
					"maxLength": 128,
					"pattern":   "^[A-Za-z0-9]+$",
					"x-radius-sensitive": true,
				},
			},
		}

		// After encryption, the value is an object that would not satisfy
		// minLength/maxLength/pattern — but normalization makes those inert.
		resourceData := map[string]any{
			"properties": map[string]any{
				"apiKey": map[string]any{
					"encrypted": "base64-ciphertext",
					"nonce":     "base64-nonce",
					"ad":        "base64-hash",
					"version":   1,
				},
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

	t.Run("non-sensitive field with wrong type still fails validation", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type": "string",
				},
				"password": map[string]any{
					"type": "string",
					"x-radius-sensitive": true,
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"name":     12345, // Wrong type - should still fail
				"password": map[string]any{"encrypted": "data", "nonce": "data"},
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resource data validation failed")
	})

	t.Run("sensitive string field rejects integer value", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"password": map[string]any{
					"type": "string",
					"x-radius-sensitive": true,
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"password": 12345, // Neither string nor object
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resource data validation failed")
	})

	t.Run("sensitive string field rejects boolean value", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"password": map[string]any{
					"type": "string",
					"x-radius-sensitive": true,
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"password": true, // Neither string nor object
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resource data validation failed")
	})

	t.Run("sensitive string field rejects array value", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"password": map[string]any{
					"type": "string",
					"x-radius-sensitive": true,
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"password": []any{"not", "valid"}, // Neither string nor object
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resource data validation failed")
	})

	t.Run("sensitive string directly on items schema accepts encrypted values", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tokens": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
						"x-radius-sensitive": true,
					},
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"tokens": []any{
					map[string]any{"encrypted": "data1", "nonce": "nonce1"},
					map[string]any{"encrypted": "data2", "nonce": "nonce2"},
				},
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

	t.Run("sensitive string directly on additionalProperties schema accepts encrypted values", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"secretMap": map[string]any{
					"type": "object",
					"additionalProperties": map[string]any{
						"type": "string",
						"x-radius-sensitive": true,
					},
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"secretMap": map[string]any{
					"key1": map[string]any{"encrypted": "data1", "nonce": "nonce1"},
					"key2": map[string]any{"encrypted": "data2", "nonce": "nonce2"},
				},
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

	t.Run("sensitive string field rejects empty object", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"password": map[string]any{
					"type": "string",
					"x-radius-sensitive": true,
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"password": map[string]any{}, // Empty object — not valid encrypted data
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resource data validation failed")
	})

	t.Run("nullable sensitive string accepts null after normalization", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"token": map[string]any{
					"type":               "string",
					"nullable":           true,
					"x-radius-sensitive": true,
				},
			},
		}

		resourceData := map[string]any{
			"properties": map[string]any{
				"token": nil,
			},
		}

		err := ValidateResourceAgainstSchema(ctx, resourceData, schema)
		require.NoError(t, err)
	})

}
