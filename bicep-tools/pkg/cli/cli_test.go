package cli

import (
	"os"
	"testing"
)

const testManifestYAML = `name: MyCompany.Resources
types:
  testResources:
    apiVersions:
      '2025-01-01-preview':
        schema:
          type: object
          properties:
            name:
              type: string
              description: "Resource name"
            count:
              type: integer
              description: "Resource count"
        capabilities: ['Recipes']`

func TestGenerator_GenerateFromString(t *testing.T) {
	generator := NewGenerator()
	
	result, err := generator.GenerateFromString(testManifestYAML)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected result to not be nil")
	}
	
	if result.TypesContent == "" {
		t.Error("Expected types content to not be empty")
	}
	
	if result.IndexContent == "" {
		t.Error("Expected index content to not be empty")
	}
	
	if result.DocumentationContent == "" {
		t.Error("Expected documentation content to not be empty")
	}
	
	// Basic validation that the types content is valid JSON
	if result.TypesContent[0] != '[' {
		t.Error("Expected types content to start with '['")
	}
	
	// Basic validation that the index content is valid JSON
	if result.IndexContent[0] != '{' {
		t.Error("Expected index content to start with '{'")
	}
	
	// Basic validation that the documentation content is markdown
	if len(result.DocumentationContent) < 10 {
		t.Error("Expected documentation content to have reasonable length")
	}
}

func TestGenerator_GenerateFromFile(t *testing.T) {
	// Create a temporary file with test manifest
	tempFile, err := os.CreateTemp("", "test-manifest-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	
	if _, err := tempFile.WriteString(testManifestYAML); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()
	
	generator := NewGenerator()
	
	result, err := generator.GenerateFromFile(tempFile.Name())
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected result to not be nil")
	}
	
	if result.TypesContent == "" {
		t.Error("Expected types content to not be empty")
	}
	
	if result.IndexContent == "" {
		t.Error("Expected index content to not be empty")
	}
	
	if result.DocumentationContent == "" {
		t.Error("Expected documentation content to not be empty")
	}
}

func TestGenerator_GenerateFromFile_FileNotExists(t *testing.T) {
	generator := NewGenerator()
	
	_, err := generator.GenerateFromFile("non-existent-file.yaml")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestGenerator_GenerateFromString_InvalidYAML(t *testing.T) {
	generator := NewGenerator()
	
	invalidYAML := `invalid: yaml: content: [`
	
	_, err := generator.GenerateFromString(invalidYAML)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestGenerator_GenerateFromString_InvalidManifest(t *testing.T) {
	generator := NewGenerator()
	
	// Missing required fields
	invalidManifest := `name: MyCompany.Resources`
	
	_, err := generator.GenerateFromString(invalidManifest)
	if err == nil {
		t.Error("Expected error for invalid manifest, got nil")
	}
}