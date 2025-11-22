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
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// Constants for reserved property names
const (
	reservedPropApplication = "application"
	reservedPropEnvironment = "environment"
	reservedPropStatus      = "status"
	reservedPropRecipe      = "recipe"
	reservedPropConnections = "connections"
)

// joinPath concatenates two path segments with a dot separator for property path tracking.
func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	if child == "" {
		return parent
	}
	return parent + "." + child
}

// isPlatformOptionsPath checks if a path ends with "platformOptions" (last segment after splitting by dots).
func isPlatformOptionsPath(path string) bool {
	if path == "" {
		return false
	}
	segments := strings.Split(path, ".")
	return segments[len(segments)-1] == "platformOptions"
}

// isPlatformOptionsAdditionalPropertiesPath checks if a path points to the
// additionalProperties schema under a platformOptions property.
func isPlatformOptionsAdditionalPropertiesPath(path string) bool {
	if path == "" {
		return false
	}
	segments := strings.Split(path, ".")
	if len(segments) < 2 {
		return false
	}
	return segments[len(segments)-2] == "platformOptions" && segments[len(segments)-1] == "additionalProperties"
}

// normalizePlatformOptionsAny rewrites type: any occurrences that sit beneath
// platformOptions.additionalProperties into spec-compliant schemas so the
// OpenAPI validator accepts them while keeping the rest of the document intact.
func normalizePlatformOptionsAny(schema *openapi3.Schema) {
	normalizePlatformOptionsAnyWithPath(schema, "")
}

func normalizePlatformOptionsAnyWithPath(schema *openapi3.Schema, path string) {
	if schema == nil {
		return
	}

	if isPlatformOptionsAdditionalPropertiesPath(path) && schema.Type != nil {
		for _, t := range *schema.Type {
			if strings.EqualFold(t, "any") {
				schema.Type = nil
				break
			}
		}
	}

	for propName, propRef := range schema.Properties {
		if propRef == nil || propRef.Value == nil {
			continue
		}
		normalizePlatformOptionsAnyWithPath(propRef.Value, joinPath(path, propName))
	}

	if schema.AdditionalProperties.Schema != nil && schema.AdditionalProperties.Schema.Value != nil {
		normalizePlatformOptionsAnyWithPath(schema.AdditionalProperties.Schema.Value, joinPath(path, "additionalProperties"))
	}

	if schema.Items != nil && schema.Items.Value != nil {
		normalizePlatformOptionsAnyWithPath(schema.Items.Value, joinPath(path, "items"))
	}
}

// isUnconstrainedSchema returns true if a schema has no type restrictions and accepts any value.
func isUnconstrainedSchema(schema *openapi3.Schema) bool {
	if schema == nil {
		return false
	}
	if schema.Type != nil && len(*schema.Type) > 0 {
		return false
	}
	if schema.Items != nil {
		return false
	}
	if len(schema.Properties) > 0 {
		return false
	}
	if schema.AdditionalProperties.Schema != nil {
		return false
	}
	if schema.AdditionalProperties.Has != nil && *schema.AdditionalProperties.Has {
		return false
	}
	if len(schema.AllOf) > 0 || len(schema.AnyOf) > 0 || len(schema.OneOf) > 0 {
		return false
	}
	if schema.Not != nil {
		return false
	}
	return true
}

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

	// Rewrite Radius-specific allowances (type:any under platformOptions additionalProperties)
	// into OpenAPI-compliant shapes before invoking the generic validator.
	normalizePlatformOptionsAny(schema)

	var errors ValidationErrors

	// Validate the schema using OpenAPI loader
	err := v.validateSchemaWithOpenAPI(schema)
	if err != nil {
		errors.Add(NewSchemaError("", fmt.Sprintf("OpenAPI schema validation failed: %s", err.Error())))
	}

	// Check reserved property constraints at root level only
	if err := v.checkReservedProperties(schema); err != nil {
		// If it's already a ValidationErrors collection, merge it
		if valErrs, ok := err.(*ValidationErrors); ok {
			for _, ve := range valErrs.Errors {
				errors.Add(ve)
			}
		} else if valErr, ok := err.(*ValidationError); ok {
			errors.Add(valErr)
		} else {
			errors.Add(NewConstraintError("", err.Error()))
		}
	}

	// Check Radius-specific constraints
	if err := v.validateRadiusConstraints(schema); err != nil {
		// If it's already a ValidationErrors collection, merge it
		if valErrs, ok := err.(*ValidationErrors); ok {
			for _, ve := range valErrs.Errors {
				errors.Add(ve)
			}
		} else if valErr, ok := err.(*ValidationError); ok {
			errors.Add(valErr)
		} else {
			errors.Add(NewConstraintError("", fmt.Sprintf("schema violates Radius constraints: %s", err.Error())))
		}
	}

	if errors.HasErrors() {
		return &errors
	}

	return nil
}

// ValidateConstraints checks if a schema meets Radius-specific constraints
func (v *Validator) validateRadiusConstraints(schema *openapi3.Schema) error {
	return v.validateRadiusConstraintsWithPath(schema, "")
}

func (v *Validator) validateRadiusConstraintsWithPath(schema *openapi3.Schema, path string) error {
	var errors ValidationErrors

	// Check for prohibited features
	if err := v.checkProhibitedFeatures(schema); err != nil {
		if valErr, ok := err.(*ValidationError); ok {
			errors.Add(valErr)
		} else {
			errors.Add(NewConstraintError("", err.Error()))
		}
	}

	// Check for prohibited $ref usage throughout the schema
	// We need to wrap the schema in a SchemaRef to use our recursive function
	schemaRef := &openapi3.SchemaRef{Value: schema}
	if err := v.checkRefUsage(schemaRef, path); err != nil {
		if valErr, ok := err.(*ValidationError); ok {
			errors.Add(valErr)
		} else {
			errors.Add(NewConstraintError("", err.Error()))
		}
	}

	// Check object property constraints
	if err := v.checkObjectPropertyConstraints(schema, path); err != nil {
		if valErr, ok := err.(*ValidationError); ok {
			errors.Add(valErr)
		} else {
			errors.Add(NewConstraintError("", err.Error()))
		}
	}

	// Validate type constraints
	if err := v.validateTypeConstraints(schema, path); err != nil {
		if valErr, ok := err.(*ValidationError); ok {
			errors.Add(valErr)
		} else {
			errors.Add(NewConstraintError("", err.Error()))
		}
	}

	// Recursively validate object properties
	if schema.Properties != nil {
		for propName, propRef := range schema.Properties {
			if propRef == nil {
				field := joinPath(path, propName)
				errors.Add(NewSchemaError(field, "property schema is nil"))
				continue
			}

			// If this is a reference (has Ref), we don't need to validate the Value
			// because the actual schema is defined elsewhere
			if propRef.Ref != "" {
				// The $ref validation is already handled by checkRefUsage above
				continue
			}

			if propRef.Value == nil {
				field := joinPath(path, propName)
				errors.Add(NewSchemaError(field, "property schema is nil"))
				continue
			}

			if err := v.validateRadiusConstraintsWithPath(propRef.Value, joinPath(path, propName)); err != nil {
				// Add property context to error
				if valErrs, ok := err.(*ValidationErrors); ok {
					for _, ve := range valErrs.Errors {
						// Clone the error to avoid modifying the original
						contextualErr := &ValidationError{
							Type:    ve.Type,
							Field:   ve.Field,
							Message: ve.Message,
						}
						baseField := joinPath(path, propName)
						if contextualErr.Field != "" {
							contextualErr.Field = joinPath(baseField, contextualErr.Field)
						} else {
							contextualErr.Field = baseField
						}
						errors.Add(contextualErr)
					}
				} else if valErr, ok := err.(*ValidationError); ok {
					// Clone the error to avoid modifying the original
					contextualErr := &ValidationError{
						Type:    valErr.Type,
						Field:   valErr.Field,
						Message: valErr.Message,
					}
					baseField := joinPath(path, propName)
					if contextualErr.Field != "" {
						contextualErr.Field = joinPath(baseField, contextualErr.Field)
					} else {
						contextualErr.Field = baseField
					}
					errors.Add(contextualErr)
				} else {
					errors.Add(NewSchemaError(joinPath(path, propName), err.Error()))
				}
			}
		}
	}

	// Also validate additionalProperties if present
	if addPropSchema := schema.AdditionalProperties.Schema; addPropSchema != nil {
		if addPropSchema.Ref != "" {
			// The $ref validation is already handled by checkRefUsage above
		} else if addPropSchema.Value != nil {
			if err := v.validateRadiusConstraintsWithPath(addPropSchema.Value, joinPath(path, "additionalProperties")); err != nil {
				// Add context to error
				if valErrs, ok := err.(*ValidationErrors); ok {
					for _, ve := range valErrs.Errors {
						// Clone the error to avoid modifying the original
						contextualErr := &ValidationError{
							Type:    ve.Type,
							Field:   joinPath(joinPath(path, "additionalProperties"), ve.Field),
							Message: ve.Message,
						}
						errors.Add(contextualErr)
					}
				} else if valErr, ok := err.(*ValidationError); ok {
					// Clone the error to avoid modifying the original
					contextualErr := &ValidationError{
						Type:    valErr.Type,
						Field:   joinPath(joinPath(path, "additionalProperties"), valErr.Field),
						Message: valErr.Message,
					}
					errors.Add(contextualErr)
				} else {
					errors.Add(NewSchemaError(joinPath(path, "additionalProperties"), err.Error()))
				}
			}
		}
	}

	if errors.HasErrors() {
		return &errors
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
			Title:   "validateSchema",
			Version: "1.0.0",
		},
		Components: &openapi3.Components{
			Schemas: map[string]*openapi3.SchemaRef{
				"validateSchema": {Value: schema},
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
func (v *Validator) validateTypeConstraints(schema *openapi3.Schema, path string) error {
	// If type is not specified, it's valid (OpenAPI allows typeless schemas)
	if schema.Type == nil {
		return nil
	}

	supportedTypes := []string{"string", "number", "integer", "boolean", "object", "array", "enum"}

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

	// Allow 'type: any' only under platformOptions
	if typeStr == "any" && isPlatformOptionsPath(path) {
		return nil
	}

	return NewConstraintError("", fmt.Sprintf("unsupported type: %s", typeStr))
}

// checkObjectPropertyConstraints validates object-specific constraints
func (v *Validator) checkObjectPropertyConstraints(schema *openapi3.Schema, path string) error {
	// Apply this constraint to:
	// 1. Explicit object types
	// 2. Typeless schemas that have properties or additionalProperties (implicitly objects)
	// 3. BUT NOT to schemas that are explicitly typed as non-objects
	isExplicitNonObject := schema.Type != nil && !schema.Type.Is("object")
	if isExplicitNonObject {
		return nil // Skip constraint for explicitly non-object types
	}

	// Now we're dealing with either explicit objects or typeless schemas
	hasObjectFeatures := schema.Properties != nil || schema.AdditionalProperties.Has != nil || schema.AdditionalProperties.Schema != nil
	if !hasObjectFeatures {
		return nil // No object features to validate
	}

	// Check if additionalProperties is set to true (boolean true is not allowed)
	if schema.AdditionalProperties.Has != nil && *schema.AdditionalProperties.Has {
		return NewConstraintError("", "additionalProperties: true is not allowed, use a schema object instead")
	}

	// Check if both properties and additionalProperties are defined
	// Note: Empty properties map should be treated as no properties defined
	hasProperties := len(schema.Properties) > 0
	hasAdditionalProperties := schema.AdditionalProperties.Schema != nil

	if hasProperties && hasAdditionalProperties {
		return NewConstraintError("", "object schemas cannot have both 'properties' and 'additionalProperties' defined")
	}

	if hasAdditionalProperties {
		addPropsRef := schema.AdditionalProperties.Schema
		if addPropsRef != nil {
			if addPropsRef.Value == nil && addPropsRef.Ref == "" {
				return NewSchemaError(joinPath(path, "additionalProperties"), "additionalProperties schema is nil")
			}
			if addPropsRef.Value != nil && isUnconstrainedSchema(addPropsRef.Value) && !isPlatformOptionsPath(path) {
				return NewConstraintError(joinPath(path, "additionalProperties"), "additionalProperties may be type `any` only for the platformOptions property")
			}
		}
	}

	return nil
}

// checkReservedProperties validates reserved property constraints
func (v *Validator) checkReservedProperties(schema *openapi3.Schema) error {
	if schema.Properties == nil {
		return nil
	}

	var errors ValidationErrors

	for propName, propRef := range schema.Properties {
		// Check for restricted property names
		if propName == reservedPropStatus || propName == reservedPropRecipe {
			err := NewConstraintError(propName, fmt.Sprintf("property '%s' is reserved and cannot be used", propName))
			errors.Add(err)
		}

		// Check specific property type constraints
		if propName == reservedPropApplication || propName == reservedPropEnvironment {
			if propRef.Value != nil {
				if propRef.Value.Type == nil || !propRef.Value.Type.Is("string") {
					err := NewConstraintError(propName, fmt.Sprintf("property '%s' must be a string", propName))
					errors.Add(err)
				}
			}
		}

		if propName == reservedPropConnections {
			if propRef.Value != nil {
				// Check if it's an object type
				if propRef.Value.Type != nil && !propRef.Value.Type.Is("object") {
					err := NewConstraintError(propName, fmt.Sprintf("property '%s' must be a map object", reservedPropConnections))
					errors.Add(err)
				}

				// If it's an object, ensure it's map-like (must have additionalProperties)
				if propRef.Value.Type != nil && propRef.Value.Type.Is("object") {
					hasAdditionalProps := (propRef.Value.AdditionalProperties.Has != nil && *propRef.Value.AdditionalProperties.Has) ||
						propRef.Value.AdditionalProperties.Schema != nil

					if !hasAdditionalProps {
						err := NewConstraintError(propName, fmt.Sprintf("property '%s' must be a map object (use additionalProperties)", reservedPropConnections))
						errors.Add(err)
					}
				}
			}
		}
	}

	// Check that environment property is always included
	if schema.Properties != nil {
		if _, hasEnv := schema.Properties[reservedPropEnvironment]; !hasEnv {
			err := NewConstraintError(reservedPropEnvironment, fmt.Sprintf("property '%s' must be included in schema", reservedPropEnvironment))
			errors.Add(err)
		}
	}

	if errors.HasErrors() {
		return &errors
	}

	return nil
}

// ValidateResourceAgainstSchema validates resource data against an OpenAPI 3.0 schema.
// It converts the schema data to OpenAPI format, creates a minimal OpenAPI document for validation,
// and then validates the resource data against the schema using OpenAPI's built-in validation.
func ValidateResourceAgainstSchema(ctx context.Context, resourceData map[string]any, schemaData any) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	if schemaData == nil {
		// Extract resource identifier for cleaner logging
		resourceID := "unknown"
		if id, ok := resourceData["id"].(string); ok && id != "" {
			resourceID = id
		} else if name, ok := resourceData["name"].(string); ok && name != "" {
			resourceID = name
		}

		logger.V(ucplog.LevelDebug).Info("No schema data provided, skipping validation",
			"resourceID", resourceID)
		return nil // No schema to validate against

	}

	// Convert schema to OpenAPI schema format
	openAPISchema, err := ConvertToOpenAPISchema(schemaData)
	if err != nil {
		return fmt.Errorf("failed to convert schema: %w", err)
	}

	// Apply the same Radius-specific normalization used during schema registration so
	// runtime validation can accept platformOptions.additionalProperties type:any.
	normalizePlatformOptionsAny(openAPISchema)

	// Create a minimal OpenAPI document with the schema
	doc := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "validateSchema",
			Version: "1.0.0",
		},
		Components: &openapi3.Components{
			Schemas: map[string]*openapi3.SchemaRef{
				"validateSchema": {Value: openAPISchema},
			},
		},
		Paths: &openapi3.Paths{},
	}

	// Validate the document structure
	if err := doc.Validate(ctx); err != nil {
		return fmt.Errorf("resource type schema validation failed: %w", err)
	}

	// Validate the data against the schema
	schemaRef := &openapi3.SchemaRef{Value: openAPISchema}

	propertiesData, ok := resourceData["properties"]
	if !ok {
		return fmt.Errorf("resource data missing 'properties' field")
	}

	if err := schemaRef.Value.VisitJSON(propertiesData); err != nil {
		// Try to extract structured error information
		if openAPIErr, ok := err.(*openapi3.SchemaError); ok {

			// Clean up the JSON pointer for better readability
			schemaErr := openAPIErr.JSONPointer()
			fieldPath := fmt.Sprintf("%v", schemaErr)
			fieldPath = strings.Trim(fieldPath, "[]")

			message := fmt.Sprintf("Error at %q: %s", fieldPath, openAPIErr.Reason)
			return fmt.Errorf("resource data validation failed: %s", message)
		}

		return fmt.Errorf("resource data validation failed: %w", err)
	}

	return nil
}
