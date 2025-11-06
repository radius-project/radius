package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/bicep-tools/pkg/cli"
)

// TestIntegration_WithTypescriptTestData tests the Go implementation against the same
// test data used by the TypeScript implementation to ensure feature parity
func TestIntegration_WithTypescriptTestData(t *testing.T) {
	testCases := []struct {
		name         string
		manifestFile string
	}{
		{
			name:         "valid manifest",
			manifestFile: "../internal/testdata/valid.yaml",
		},
		{
			name:         "manifest with schema properties",
			manifestFile: "../internal/testdata/valid-with-schema-properties.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate using our Go implementation
			generator := cli.NewGenerator()
			result, err := generator.GenerateFromFile(tc.manifestFile)
			if err != nil {
				t.Fatalf("Failed to generate from manifest: %v", err)
			}

			// Validate that all outputs are generated
			if result.TypesContent == "" {
				t.Error("Types content should not be empty")
			}
			if result.IndexContent == "" {
				t.Error("Index content should not be empty")
			}
			if result.DocumentationContent == "" {
				t.Error("Documentation content should not be empty")
			}

			// Validate that types.json is valid JSON array
			var types []interface{}
			if err := json.Unmarshal([]byte(result.TypesContent), &types); err != nil {
				t.Errorf("Types content is not valid JSON: %v", err)
			}

			// Validate that index.json is valid JSON object
			var index map[string]interface{}
			if err := json.Unmarshal([]byte(result.IndexContent), &index); err != nil {
				t.Errorf("Index content is not valid JSON: %v", err)
			}

			// Validate index structure
			if _, ok := index["resources"]; !ok {
				t.Error("Index should contain resources")
			}
			if _, ok := index["settings"]; !ok {
				t.Error("Index should contain settings")
			}

			// Test that we can write and read back the files
			tempDir, err := os.MkdirTemp("", "integration-test-")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Write the files
			files := map[string]string{
				"types.json": result.TypesContent,
				"index.json": result.IndexContent,
				"index.md":   result.DocumentationContent,
			}

			for filename, content := range files {
				path := filepath.Join(tempDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Errorf("Failed to write %s: %v", filename, err)
				}

				// Verify file was written correctly
				readBack, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("Failed to read back %s: %v", filename, err)
				}
				if string(readBack) != content {
					t.Errorf("File %s content mismatch after write/read", filename)
				}
			}
		})
	}
}

// TestIntegration_CLIEndToEnd tests the complete CLI workflow
func TestIntegration_CLIEndToEnd(t *testing.T) {
	// Use valid test data
	manifestFile := "../internal/testdata/valid-with-schema-properties.yaml"

	// Create temporary output directory
	tempDir, err := os.MkdirTemp("", "cli-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate using CLI package (simulating command line usage)
	generator := cli.NewGenerator()
	result, err := generator.GenerateFromFile(manifestFile)
	if err != nil {
		t.Fatalf("CLI generation failed: %v", err)
	}

	// Write output files (simulating CLI file writing)
	files := map[string]string{
		"types.json": result.TypesContent,
		"index.json": result.IndexContent,
		"index.md":   result.DocumentationContent,
	}

	for filename, content := range files {
		path := filepath.Join(tempDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", filename, err)
		}
	}

	// Verify all expected files exist
	expectedFiles := []string{"types.json", "index.json", "index.md"}
	for _, filename := range expectedFiles {
		path := filepath.Join(tempDir, filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s was not created", filename)
		}
	}

	// Verify the content matches our test data expectations
	// Test specific content based on the test YAML
	var index map[string]interface{}
	indexPath := filepath.Join(tempDir, "index.json")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("Failed to read index.json: %v", err)
	}
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("Failed to parse index.json: %v", err)
	}

	// Verify resources structure
	resources, ok := index["resources"].(map[string]interface{})
	if !ok {
		t.Fatal("Index resources should be an object")
	}

	// Should contain the test resource with version
	testResourceWithVersion := "MyCompany.Resources/testResources@2025-01-01-preview"
	if _, exists := resources[testResourceWithVersion]; !exists {
		t.Errorf("Expected resource %s not found in index", testResourceWithVersion)
	}

	// Verify settings
	settings, ok := index["settings"].(map[string]interface{})
	if !ok {
		t.Fatal("Index settings should be an object")
	}

	expectedName := "radiusmycompanyresources"
	if name, ok := settings["name"].(string); !ok || name != expectedName {
		t.Errorf("Expected settings name %s, got %v", expectedName, settings["name"])
	}
}

// TestIntegration_ErrorHandling tests error scenarios
func TestIntegration_ErrorHandling(t *testing.T) {
	generator := cli.NewGenerator()

	// Test non-existent file
	_, err := generator.GenerateFromFile("non-existent-file.yaml")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}

	// Test invalid YAML
	_, err = generator.GenerateFromString("invalid: yaml: [")
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}

	// Test invalid manifest (missing required fields)
	_, err = generator.GenerateFromString("name: Test")
	if err == nil {
		t.Error("Expected error for invalid manifest")
	}
}

// BenchmarkIntegration_Generation benchmarks the complete generation process
func BenchmarkIntegration_Generation(b *testing.B) {
	manifestFile := "../internal/testdata/valid-with-schema-properties.yaml"
	generator := cli.NewGenerator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := generator.GenerateFromFile(manifestFile)
		if err != nil {
			b.Fatalf("Generation failed: %v", err)
		}
	}
}
