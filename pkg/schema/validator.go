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
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// Validator validates OpenAPI 3.0 schemas with Radius-specific constraints
type Validator struct{}

// NewValidator creates a new schema validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateSchema validates an OpenAPI 3.0 schema against Radius constraints
func (v *Validator) ValidateSchema(ctx context.Context, schema *openapi3.Schema) error {
	if schema == nil {
		return fmt.Errorf("schema cannot be nil")
	}

	// Check Radius-specific constraints
	if err := v.validateRadiusConstraints(schema); err != nil {
		return fmt.Errorf("schema violates Radius constraints: %w", err)
	}

	return nil
}

// ValidateConstraints checks if a schema meets Radius-specific constraints
func (v *Validator) validateRadiusConstraints(schema *openapi3.Schema) error {
	// Check for prohibited features
	if err := v.checkProhibitedFeatures(schema); err != nil {
		return err
	}

	// Validate type constraints
	if err := v.validateTypeConstraints(schema); err != nil {
		return err
	}

	// Recursively validate object properties
	if schema.Properties != nil {
		for propName, propRef := range schema.Properties {
			if propRef == nil || propRef.Value == nil {
				return NewSchemaError(propName, "property schema is nil")
			}
			if err := v.validateRadiusConstraints(propRef.Value); err != nil {
				// Add property context to error
				if valErr, ok := err.(*ValidationError); ok {
					if valErr.Field != "" {
						valErr.Field = propName + "." + valErr.Field
					} else {
						valErr.Field = propName
					}
					return valErr
				}
				return NewSchemaError(propName, err.Error())
			}
		}
	}

	// Also validate additionalProperties if present
	if schema.AdditionalProperties.Has != nil {
		if addPropSchema := schema.AdditionalProperties.Schema; addPropSchema != nil && addPropSchema.Value != nil {
			if err := v.validateRadiusConstraints(addPropSchema.Value); err != nil {
				// Add context to error
				if valErr, ok := err.(*ValidationError); ok {
					valErr.Field = "additionalProperties." + valErr.Field
					return valErr
				}
				return NewSchemaError("additionalProperties", err.Error())
			}
		}
	}

	return nil
}

// ConvertToOpenAPISchema converts the schema any to OpenAPI schema
func ConvertToOpenAPISchema(schemaData any) (*openapi3.Schema, error) {
	// Convert to JSON bytes first
	jsonData, err := json.Marshal(schemaData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Parse as OpenAPI schema
	var schema openapi3.Schema
	if err := json.Unmarshal(jsonData, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse as OpenAPI schema: %w", err)
	}

	return &schema, nil
}

// checkProhibitedFeatures checks for OpenAPI features not allowed in Radius
func (v *Validator) checkProhibitedFeatures(schema *openapi3.Schema) error {
	if len(schema.AllOf) > 0 {
		return NewConstraintError("", "allOf is not supported")
	}
	if len(schema.AnyOf) > 0 {
		return NewConstraintError("", "anyOf is not supported")
	}
	if len(schema.OneOf) > 0 {
		return NewConstraintError("", "oneOf is not supported")
	}
	if schema.Not != nil {
		return NewConstraintError("", "not is not supported")
	}
	if schema.Discriminator != nil {
		return NewConstraintError("", "discriminator is not supported")
	}

	return nil
}

// validateTypeConstraints ensures only supported types are used
func (v *Validator) validateTypeConstraints(schema *openapi3.Schema) error {
	// If type is not specified, it's valid (OpenAPI allows typeless schemas) - verify this
	if schema.Type == nil {
		return nil
	}

	supportedTypes := []string{"string", "number", "integer", "boolean", "object"}

	for _, supported := range supportedTypes {
		if schema.Type.Is(supported) {
			return nil
		}
	}

	return NewConstraintError("", fmt.Sprintf("unsupported type: %s", schema.Type))
}
