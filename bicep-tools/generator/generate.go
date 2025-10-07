package generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/bicep-tools/pkg/cli"
)

func RunGenerate(manifestFile, outputDir string) error {
	// Validate input file exists
	if _, err := os.Stat(manifestFile); os.IsNotExist(err) {
		return fmt.Errorf("manifest file does not exist: %s", manifestFile)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Validate output directory is writable
	testFile := filepath.Join(outputDir, ".write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("output directory is not writable: %w", err)
	}
	os.Remove(testFile)

	// Use CLI package to perform the conversion
	generator := cli.NewGenerator()
	result, err := generator.GenerateFromFile(manifestFile)
	if err != nil {
		return fmt.Errorf("failed to generate from manifest: %w", err)
	}

	// Write output files
	files := map[string]string{
		"types.json": result.TypesContent,
		"index.json": result.IndexContent,
		"index.md":   result.DocumentationContent,
	}

	for filename, content := range files {
		outputPath := filepath.Join(outputDir, filename)

		// Remove existing file if it exists
		if err := removeIfExists(outputPath); err != nil {
			return fmt.Errorf("failed to remove existing file %s: %w", outputPath, err)
		}

		fmt.Printf("Writing %s to %s\n", filename, outputPath)
		if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filename, err)
		}
	}

	fmt.Printf("Successfully generated Bicep extension files in %s\n", outputDir)
	return nil
}

func removeIfExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		return os.Remove(path)
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
