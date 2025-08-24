package manifest

import (
	"testing"
)

// Test data from the TypeScript implementation
const validYAML = `name: MyCompany.Resources
types:
  testResources:
    apiVersions:
      '2025-01-01-preview':
        schema: {}
        capabilities: ['Recipes']`

const validWithSchemaPropertiesYAML = `name: MyCompany.Resources
types:
  testResources:
    apiVersions:
      '2025-01-01-preview':
        schema:
          type: object
          properties:
            a:
              type: integer
              description: "An integer property"
            b:
              type: boolean
              description: "A boolean property"
            c:
              type: string
              description: "A string property"
            connections:
              type: object
              additionalProperties: 
                type: object
                properties:
                  source:
                    type: string
                    description: "A connection string property"
        capabilities: ['Recipes']`

func TestParseManifest_Valid(t *testing.T) {
	result, err := ParseManifest(validYAML)
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
	result, err := ParseManifest(validWithSchemaPropertiesYAML)
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
