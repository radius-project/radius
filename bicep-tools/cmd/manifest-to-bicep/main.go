package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/bicep-tools/pkg/cli"
	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
	cmd := &cobra.Command{
		Use:   "generate <manifest1> [manifest2 ...] <output>",
		Short: "Generate Bicep extension from one or more Radius Resource Provider manifests",
		Long: `Generate Bicep extension files from one or more Radius Resource Provider manifests.

This command takes YAML manifest files that define resource types and their
schemas, and generates three output files:

- types.json: Bicep type definitions
- index.json: Type index for Bicep extensions
- index.md: Markdown documentation

When multiple manifest files are provided, their resource type definitions are
merged into a single output. All manifests must share the same namespace because
the output is written to a single directory representing one namespace/apiVersion
combination. To merge types across namespaces into one Bicep extension, run this
command once per namespace and then rebuild the unified index.json over the
combined output tree (see the rebuild-index step in the build pipeline).

This supports per-type manifest files (e.g. containers.yaml, routes.yaml) that
each define a single resource type within the same namespace.

The last positional argument is always the output directory; all preceding
arguments are manifest files.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestFiles := args[:len(args)-1]
			outputDir := args[len(args)-1]
			return RunGenerate(manifestFiles, outputDir)
		},
	}

	return cmd
}

// RunGenerate generates Bicep extension files (types.json, index.json, index.md) from one or more
// manifest files. When multiple manifests are provided, they must share the same namespace and their
// resource type definitions are merged before generation. This supports per-type manifest files
// (e.g. containers.yaml, routes.yaml) where each file defines a single resource type.
//
// The same-namespace restriction exists because the output is written to a single directory
// representing one namespace/apiVersion. Cross-namespace unification into one Bicep extension
// is handled separately by the rebuild-index step, which walks the full output tree and builds
// a unified index.json.
func RunGenerate(manifestFiles []string, outputDir string) error {
	if len(manifestFiles) == 0 {
		return fmt.Errorf("at least one manifest file is required")
	}

	// Validate all input files exist
	for _, f := range manifestFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return fmt.Errorf("manifest file does not exist: %s", f)
		}
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

	var result *cli.GenerationResult
	var err error

	if len(manifestFiles) == 1 {
		// Single manifest - use directly without merging.
		result, err = generator.GenerateFromFile(manifestFiles[0])
		if err != nil {
			return fmt.Errorf("failed to generate from manifest: %w", err)
		}
	} else {
		// Multiple manifests - merge their Types maps into a single manifest, then generate.
		merged, mergeErr := mergeManifestFiles(manifestFiles)
		if mergeErr != nil {
			return mergeErr
		}
		result, err = generator.GenerateFromString(merged)
		if err != nil {
			return fmt.Errorf("failed to generate from merged manifests: %w", err)
		}
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

// mergeManifestFiles reads multiple manifest YAML files, validates they share
// the same namespace, merges their Types maps, and returns a single combined
// YAML string suitable for GenerateFromString.
func mergeManifestFiles(paths []string) (string, error) {
	var namespace string
	allTypes := make(map[string]manifest.ResourceType)

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("failed to read manifest file %s: %w", p, err)
		}

		provider, err := manifest.ParseManifest(string(data))
		if err != nil {
			return "", fmt.Errorf("failed to parse manifest %s: %w", p, err)
		}

		if namespace == "" {
			namespace = provider.Namespace
		} else if provider.Namespace != namespace {
			return "", fmt.Errorf("all manifests must share the same namespace: got %q (from %s) and %q", provider.Namespace, p, namespace)
		}

		for typeName, typeDef := range provider.Types {
			if _, exists := allTypes[typeName]; exists {
				return "", fmt.Errorf("duplicate resource type %q found in %s", typeName, p)
			}
			allTypes[typeName] = typeDef
		}
	}

	// Re-serialize as YAML so GenerateFromString can parse it.
	merged := manifest.ResourceProvider{
		Namespace: namespace,
		Types:     allTypes,
	}

	out, err := yaml.Marshal(&merged)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged manifest: %w", err)
	}
	return string(out), nil
}

func removeIfExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		return os.Remove(path)
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
