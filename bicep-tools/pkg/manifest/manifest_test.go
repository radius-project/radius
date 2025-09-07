package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest_Valid(t *testing.T) {
	testdataPath := filepath.Join("../../internal/testdata", "valid.yaml")
	yamlContent, err := os.ReadFile(testdataPath)
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", testdataPath, err)
	}
	result, err := ParseManifest(string(yamlContent))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Namespace != "MyCompany.Resources" {
		t.Errorf("Expected name 'MyCompany.Resources', got: %s", result.Namespace)
	}

	testResources, exists := result.Types["testResources"]
	if !exists {
		t.Fatal("Expected 'testResources' type to exist")
	}

	apiVersion, exists := testResources.APIVersions["2025-01-01-preview"]
	if !exists {
		t.Fatal("Expected '2025-01-01-preview' API version to exist")
	}

	if len(apiVersion.Capabilities) != 1 || apiVersion.Capabilities[0] != "Recipes" {
		t.Errorf("Expected capabilities to be ['Recipes'], got: %v", apiVersion.Capabilities)
	}
}

func TestParseManifest_WithSchemaProperties(t *testing.T) {
	testdataPath := filepath.Join("../../internal/testdata", "valid-with-schema-properties.yaml")
	yamlContent, err := os.ReadFile(testdataPath)
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", testdataPath, err)
	}
	result, err := ParseManifest(string(yamlContent))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	schema := result.Types["testResources"].APIVersions["2025-01-01-preview"].Schema

	// Check that properties were parsed correctly
	if schema.Properties == nil {
		t.Fatal("Expected schema properties to exist")
	}

	// Test integer property
	aProp, exists := schema.Properties["a"]
	if !exists {
		t.Fatal("Expected property 'a' to exist")
	}
	if aProp.Type != "integer" {
		t.Errorf("Expected property 'a' type to be 'integer', got: %s", aProp.Type)
	}
	if aProp.Description == nil || *aProp.Description != "An integer property" {
		t.Errorf("Expected property 'a' description to be 'An integer property', got: %v", aProp.Description)
	}

	// Test boolean property
	bProp, exists := schema.Properties["b"]
	if !exists {
		t.Fatal("Expected property 'b' to exist")
	}
	if bProp.Type != "boolean" {
		t.Errorf("Expected property 'b' type to be 'boolean', got: %s", bProp.Type)
	}

	// Test string property
	cProp, exists := schema.Properties["c"]
	if !exists {
		t.Fatal("Expected property 'c' to exist")
	}
	if cProp.Type != "string" {
		t.Errorf("Expected property 'c' type to be 'string', got: %s", cProp.Type)
	}

	// Test object property
	connectionsProp, exists := schema.Properties["connections"]
	if !exists {
		t.Fatal("Expected property 'connections' to exist")
	}
	if connectionsProp.Type != "object" {
		t.Errorf("Expected property 'connections' type to be 'object', got: %s", connectionsProp.Type)
	}
}

func TestParseManifest_InvalidYAML(t *testing.T) {
	invalidYAML := `invalid: yaml: content: [`

	_, err := ParseManifest(invalidYAML)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestParseManifest_MissingName(t *testing.T) {
	missingNameYAML := `types:
  testResources:
    apiVersions:
      '2025-01-01-preview':
        schema: {}`

	_, err := ParseManifest(missingNameYAML)
	if err == nil {
		t.Error("Expected error for missing name, got nil")
	}
}

func TestParseManifest_MissingTypes(t *testing.T) {
	missingTypesYAML := `name: MyCompany.Resources`

	_, err := ParseManifest(missingTypesYAML)
	if err == nil {
		t.Error("Expected error for missing types, got nil")
	}
}

func TestResourceProvider_Validate(t *testing.T) {
	// Test valid provider
	validProvider := &ResourceProvider{
		Namespace: "Test.Provider",
		Types: map[string]ResourceType{
			"testType": {
				APIVersions: map[string]APIVersion{
					"2025-01-01": {
						Schema: Schema{
							Type: "object",
						},
					},
				},
			},
		},
	}

	if err := validProvider.Validate(); err != nil {
		t.Errorf("Expected valid provider to pass validation, got: %v", err)
	}

	// Test invalid provider (empty name)
	invalidProvider := &ResourceProvider{
		Namespace: "",
		Types: map[string]ResourceType{
			"testType": {
				APIVersions: map[string]APIVersion{
					"2025-01-01": {
						Schema: Schema{Type: "object"},
					},
				},
			},
		},
	}

	if err := invalidProvider.Validate(); err == nil {
		t.Error("Expected invalid provider to fail validation")
	}
}

func TestSchema_Validate(t *testing.T) {
	// Test valid schema types
	validTypes := []string{"string", "object", "integer", "boolean", "any"}
	for _, schemaType := range validTypes {
		schema := Schema{Type: schemaType}
		if err := schema.Validate("test"); err != nil {
			t.Errorf("Expected schema type '%s' to be valid, got error: %v", schemaType, err)
		}
	}

	// Test invalid schema type
	invalidSchema := Schema{Type: "invalid"}
	if err := invalidSchema.Validate("test"); err == nil {
		t.Error("Expected invalid schema type to fail validation")
	}
}

func TestParseManifest_WithEnumTypes(t *testing.T) {
	input := `
name: MyCompany.Resources
types:
  testResources:
    apiVersions:
      '2025-01-01-preview':
        schema:
          type: object
          properties:
            status:
              type: enum
              enum: ['active', 'inactive', 'pending']
              description: "The status of the resource"
            mode:
              type: string
              enum: ['development', 'production']
              description: "Deployment mode"
        capabilities: ['Recipes']
`

	result, err := ParseManifest(input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	schema := result.Types["testResources"].APIVersions["2025-01-01-preview"].Schema

	// Test explicit enum type
	statusProp, exists := schema.Properties["status"]
	if !exists {
		t.Fatal("Expected property 'status' to exist")
	}
	if statusProp.Type != "enum" {
		t.Errorf("Expected status type to be 'enum', got: %s", statusProp.Type)
	}
	expectedEnumValues := []string{"active", "inactive", "pending"}
	if len(statusProp.Enum) != len(expectedEnumValues) {
		t.Errorf("Expected %d enum values, got %d", len(expectedEnumValues), len(statusProp.Enum))
	}
	for i, expected := range expectedEnumValues {
		if statusProp.Enum[i] != expected {
			t.Errorf("Expected enum value %d to be '%s', got '%s'", i, expected, statusProp.Enum[i])
		}
	}
	if statusProp.Description == nil || *statusProp.Description != "The status of the resource" {
		t.Errorf("Expected status description to be 'The status of the resource', got: %v", statusProp.Description)
	}

	// Test string with enum constraint
	modeProp, exists := schema.Properties["mode"]
	if !exists {
		t.Fatal("Expected property 'mode' to exist")
	}
	if modeProp.Type != "string" {
		t.Errorf("Expected mode type to be 'string', got: %s", modeProp.Type)
	}
	expectedModeValues := []string{"development", "production"}
	if len(modeProp.Enum) != len(expectedModeValues) {
		t.Errorf("Expected %d enum values, got %d", len(expectedModeValues), len(modeProp.Enum))
	}
	for i, expected := range expectedModeValues {
		if modeProp.Enum[i] != expected {
			t.Errorf("Expected enum value %d to be '%s', got '%s'", i, expected, modeProp.Enum[i])
		}
	}
	if modeProp.Description == nil || *modeProp.Description != "Deployment mode" {
		t.Errorf("Expected mode description to be 'Deployment mode', got: %v", modeProp.Description)
	}
}

func TestParseManifest_WithAdditionalProperties(t *testing.T) {
	input := `
name: MyCompany.Resources
types:
  testResources:
    apiVersions:
      '2025-01-01-preview':
        schema:
          type: object
          properties:
            connections:
              type: object
              additionalProperties:
                type: object
                properties:
                  endpoint:
                    type: string
                    description: "Connection endpoint"
                  status:
                    type: enum
                    enum: ['active', 'inactive']
            metadata:
              type: object
              additionalProperties: any
        capabilities: ['Recipes']
`

	result, err := ParseManifest(input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	schema := result.Types["testResources"].APIVersions["2025-01-01-preview"].Schema

	// Test object with structured additionalProperties
	connectionsProp, exists := schema.Properties["connections"]
	if !exists {
		t.Fatal("Expected property 'connections' to exist")
	}
	if connectionsProp.Type != "object" {
		t.Errorf("Expected connections type to be 'object', got: %s", connectionsProp.Type)
	}
	if connectionsProp.AdditionalProperties == nil {
		t.Fatal("Expected connections additionalProperties to be defined")
	}
	if connectionsProp.AdditionalProperties.Type != "object" {
		t.Errorf("Expected connections additionalProperties type to be 'object', got: %s", connectionsProp.AdditionalProperties.Type)
	}

	// Check nested properties in additionalProperties
	endpointProp, exists := connectionsProp.AdditionalProperties.Properties["endpoint"]
	if !exists {
		t.Fatal("Expected additionalProperties to have 'endpoint' property")
	}
	if endpointProp.Type != "string" {
		t.Errorf("Expected endpoint type to be 'string', got: %s", endpointProp.Type)
	}

	statusProp, exists := connectionsProp.AdditionalProperties.Properties["status"]
	if !exists {
		t.Fatal("Expected additionalProperties to have 'status' property")
	}
	if statusProp.Type != "enum" {
		t.Errorf("Expected status type to be 'enum', got: %s", statusProp.Type)
	}

	// Test object with "any" additionalProperties
	metadataProp, exists := schema.Properties["metadata"]
	if !exists {
		t.Fatal("Expected property 'metadata' to exist")
	}
	if metadataProp.Type != "object" {
		t.Errorf("Expected metadata type to be 'object', got: %s", metadataProp.Type)
	}
	if metadataProp.AdditionalProperties == nil {
		t.Fatal("Expected metadata additionalProperties to be defined")
	}
	if metadataProp.AdditionalProperties.Type != "any" {
		t.Errorf("Expected metadata additionalProperties type to be 'any', got: %s", metadataProp.AdditionalProperties.Type)
	}
}

func TestParseManifest_AdditionalPropertiesTrue(t *testing.T) {
	input := `
namespace: MyCompany.Resources
types:
  testResources:
    apiVersions:
      '2025-01-01-preview':
        schema:
          type: object
          properties:
            metadata:
              type: object
              additionalProperties: true
        capabilities: ['Recipes']
`

	result, err := ParseManifest(input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	schema := result.Types["testResources"].APIVersions["2025-01-01-preview"].Schema
	metadataProp, exists := schema.Properties["metadata"]
	if !exists {
		t.Fatal("Expected property 'metadata' to exist")
	}
	if metadataProp.Type != "object" {
		t.Errorf("Expected metadata type to be 'object', got: %s", metadataProp.Type)
	}
	// Note: This test assumes your Schema struct can handle boolean additionalProperties
	// You may need to adjust based on how you've implemented the AdditionalProperties field
}

func TestParseManifest_AdditionalPropertiesAnyType(t *testing.T) {
	input := `
namespace: MyCompany.Resources
types:
  testResources:
    apiVersions:
      '2025-01-01-preview':
        schema:
          type: object
          properties:
            mymap:
              type: object
              additionalProperties:
                type: any
                description: "A map of key-value pairs"
        capabilities: ['Recipes']
`

	result, err := ParseManifest(input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	schema := result.Types["testResources"].APIVersions["2025-01-01-preview"].Schema
	mymapProp, exists := schema.Properties["mymap"]
	if !exists {
		t.Fatal("Expected property 'mymap' to exist")
	}
	if mymapProp.Type != "object" {
		t.Errorf("Expected mymap type to be 'object', got: %s", mymapProp.Type)
	}
	if mymapProp.AdditionalProperties == nil {
		t.Fatal("Expected mymap additionalProperties to be defined")
	}
	if mymapProp.AdditionalProperties.Type != "any" {
		t.Errorf("Expected mymap additionalProperties type to be 'any', got: %s", mymapProp.AdditionalProperties.Type)
	}
	if mymapProp.AdditionalProperties.Description == nil || *mymapProp.AdditionalProperties.Description != "A map of key-value pairs" {
		t.Errorf("Expected description to be 'A map of key-value pairs', got: %v", mymapProp.AdditionalProperties.Description)
	}
}
