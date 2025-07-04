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
	// If no schema is provided, it is valid
	if schema == nil {
		return nil
	}

	// Validate the schema using OpenAPI loader
	err := v.validateSchemaWithOpenAPI(schema)
	if err != nil {
		return fmt.Errorf("OpenAPI schema validation failed: %w", err)
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

	// Check for prohibited $ref usage throughout the schema
	// We need to wrap the schema in a SchemaRef to use our recursive function
	schemaRef := &openapi3.SchemaRef{Value: schema}
	if err := v.checkRefUsage(schemaRef, ""); err != nil {
		return err
	}

	// Check object property constraints
	if err := v.checkObjectPropertyConstraints(schema); err != nil {
		return err
	}

	// Validate type constraints
	if err := v.validateTypeConstraints(schema); err != nil {
		return err
	}

	// Recursively validate object properties
	if schema.Properties != nil {
		for propName, propRef := range schema.Properties {
			if propRef == nil {
				return NewSchemaError(propName, "property schema is nil")
			}

			// If this is a reference (has Ref), we don't need to validate the Value
			// because the actual schema is defined elsewhere
			if propRef.Ref != "" {
				// The $ref validation is already handled by checkRefUsage above
				continue
			}

			if propRef.Value == nil {
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
		if addPropSchema := schema.AdditionalProperties.Schema; addPropSchema != nil {
			if addPropSchema.Ref != "" {
				// The $ref validation is already handled by checkRefUsage above
				return nil
			}

			if addPropSchema.Value != nil {
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

// validateSchemaWithOpenAPI validates schema data by creating a minimal OpenAPI document
// and using the library's built-in validation which includes format validation
func (v *Validator) validateSchemaWithOpenAPI(schema *openapi3.Schema) error {
	ctx := context.Background()

	// Create a minimal OpenAPI document
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "temp",
			Version: "1.0.0",
		},
		Components: &openapi3.Components{
			Schemas: map[string]*openapi3.SchemaRef{
				"temp": {Value: schema},
			},
		},
		Paths: &openapi3.Paths{}, // Required field, even if empty
	}

	// This validates the entire document including formats, references, and schema structure
	if err := doc.Validate(ctx); err != nil {
		return fmt.Errorf("OpenAPI document validation failed: %w", err)
	}

	return nil
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

// isInternalRef checks if a $ref is an internal reference within the same document
func (v *Validator) isInternalRef(ref string) bool {
	// Internal references start with "#/" which means they reference within the same document
	// Examples of internal refs:
	// - "#/components/schemas/MySchema"
	// - "#/definitions/SomeType"
	// External references would be:
	// - "other-file.yaml#/components/schemas/MySchema"
	// - "https://example.com/schema.json#/MySchema"
	// - "relative/path/schema.yaml#/MySchema"
	return len(ref) > 0 && ref[0] == '#'
}

// checkRefUsage recursively checks for prohibited $ref usage in SchemaRef instances
func (v *Validator) checkRefUsage(schemaRef *openapi3.SchemaRef, fieldPath string) error {
	if schemaRef == nil {
		return nil
	}

	// Check if this SchemaRef contains a $ref
	if schemaRef.Ref != "" {
		// Allow internal references, but prohibit external ones
		if !v.isInternalRef(schemaRef.Ref) {
			return NewConstraintError(fieldPath, "external $ref references are not supported, only internal references starting with '#/' are allowed")
		}
	}

	// If there's no value, we're done
	if schemaRef.Value == nil {
		return nil
	}

	schema := schemaRef.Value

	// Check properties
	if schema.Properties != nil {
		for propName, propRef := range schema.Properties {
			propPath := fieldPath
			if propPath != "" {
				propPath += "."
			}
			propPath += propName

			if err := v.checkRefUsage(propRef, propPath); err != nil {
				return err
			}
		}
	}

	// Check additionalProperties
	if schema.AdditionalProperties.Schema != nil {
		addPropPath := fieldPath
		if addPropPath != "" {
			addPropPath += "."
		}
		addPropPath += "additionalProperties"

		if err := v.checkRefUsage(schema.AdditionalProperties.Schema, addPropPath); err != nil {
			return err
		}
	}

	// Check array items
	if schema.Items != nil {
		itemsPath := fieldPath
		if itemsPath != "" {
			itemsPath += "."
		}
		itemsPath += "items"

		if err := v.checkRefUsage(schema.Items, itemsPath); err != nil {
			return err
		}
	}

	// Check allOf, anyOf, oneOf (even though they're prohibited, we should check for refs)
	for i, schemaRef := range schema.AllOf {
		allOfPath := fmt.Sprintf("%s.allOf[%d]", fieldPath, i)
		if err := v.checkRefUsage(schemaRef, allOfPath); err != nil {
			return err
		}
	}

	for i, schemaRef := range schema.AnyOf {
		anyOfPath := fmt.Sprintf("%s.anyOf[%d]", fieldPath, i)
		if err := v.checkRefUsage(schemaRef, anyOfPath); err != nil {
			return err
		}
	}

	for i, schemaRef := range schema.OneOf {
		oneOfPath := fmt.Sprintf("%s.oneOf[%d]", fieldPath, i)
		if err := v.checkRefUsage(schemaRef, oneOfPath); err != nil {
			return err
		}
	}

	// Check not
	if schema.Not != nil {
		notPath := fieldPath
		if notPath != "" {
			notPath += "."
		}
		notPath += "not"

		if err := v.checkRefUsage(schema.Not, notPath); err != nil {
			return err
		}
	}

	return nil
}

// validateTypeConstraints ensures only supported types are used
func (v *Validator) validateTypeConstraints(schema *openapi3.Schema) error {
	// If type is not specified, it's valid (OpenAPI allows typeless schemas)
	if schema.Type == nil {
		return nil
	}

	supportedTypes := []string{"string", "number", "integer", "boolean", "object"}

	for _, supported := range supportedTypes {
		if schema.Type.Is(supported) {
			return nil
		}
	}

	// Get the actual type string from the Types slice
	var typeStr string
	if len(*schema.Type) > 0 {
		typeStr = (*schema.Type)[0]
	} else {
		typeStr = "unknown"
	}

	return NewConstraintError("", fmt.Sprintf("unsupported type: %s", typeStr))
}

// checkObjectPropertyConstraints validates object-specific constraints
func (v *Validator) checkObjectPropertyConstraints(schema *openapi3.Schema) error {
	// Apply this constraint to:
	// 1. Explicit object types
	// 2. Typeless schemas that have properties or additionalProperties (implicitly objects)
	// 3. BUT NOT to schemas that are explicitly typed as non-objects
	isExplicitNonObject := schema.Type != nil && !schema.Type.Is("object")
	if isExplicitNonObject {
		return nil // Skip constraint for explicitly non-object types
	}

	// Now we're dealing with either explicit objects or typeless schemas
	hasObjectFeatures := schema.Properties != nil || schema.AdditionalProperties.Has != nil
	if !hasObjectFeatures {
		return nil // No object features to validate
	}

	// Check if both properties and additionalProperties are defined
	// Note: Empty properties map should be treated as no properties defined
	hasProperties := len(schema.Properties) > 0
	hasAdditionalProperties := (schema.AdditionalProperties.Has != nil && *schema.AdditionalProperties.Has) ||
		schema.AdditionalProperties.Schema != nil

	if hasProperties && hasAdditionalProperties {
		return NewConstraintError("", "object schemas cannot have both 'properties' and 'additionalProperties' defined")
	}

	return nil
}
