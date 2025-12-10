package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/bicep-tools/pkg/cli"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cobra.CheckErr(newRootCommand().Execute())
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest-to-bicep",
		Short: "Generate Bicep extension from Radius Resource Provider manifest",
		Long: `manifest-to-bicep is a CLI tool that converts Radius Resource Provider 
manifests (YAML) into Bicep extension files (types.json, index.json, index.md).

This tool helps you create Bicep extensions for your Radius applications by 
automatically generating the necessary type definitions and documentation 
from your resource provider manifest files.`,
		SilenceUsage: true,
	}

	// Add version command
	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("manifest-to-bicep version %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("built: %s\n", date)
		},
	})

	// Add generate command
	cmd.AddCommand(newGenerateCommand())

	return cmd
}

func newGenerateCommand() *cobra.Command {
	var manifestFile string
	var outputDir string

	cmd := &cobra.Command{
		Use:   "generate <manifest> <output>",
		Short: "Generate Bicep extension from Radius Resource Provider manifest",
		Long: `Generate Bicep extension files from a Radius Resource Provider manifest.

This command takes a YAML manifest file that defines resource types and their 
schemas, and generates three output files:

- types.json: Bicep type definitions
- index.json: Type index for Bicep extensions  
- index.md: Markdown documentation

The manifest file should be a YAML file that follows the Radius Resource Provider
manifest format with resource type definitions and API versions.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestFile = args[0]
			outputDir = args[1]

			return RunGenerate(manifestFile, outputDir)
		},
	}

	return cmd
}

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
