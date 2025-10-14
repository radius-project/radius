package cli

import (
	"fmt"
	"os"

	"github.com/radius-project/radius/bicep-tools/pkg/converter"
	"github.com/radius-project/radius/bicep-tools/pkg/manifest"
)

// Generator handles the generation of Bicep extensions from manifests
type Generator struct{}

// GenerationResult represents the result of generating a Bicep extension
type GenerationResult struct {
	TypesContent         string
	IndexContent         string
	DocumentationContent string
}

// NewGenerator creates a new Generator instance
func NewGenerator() *Generator {
	return &Generator{}
}

// GenerateFromFile generates Bicep extension files from a manifest file path
func (g *Generator) GenerateFromFile(manifestPath string) (*GenerationResult, error) {
	// Read the manifest file
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	// Parse the manifest
	provider, err := manifest.ParseManifest(string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate the manifest
	if err := provider.Validate(); err != nil {
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}

	// Convert the manifest to Bicep types
	conversionResult, err := converter.Convert(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to convert manifest: %w", err)
	}

	return &GenerationResult{
		TypesContent:         conversionResult.TypesContent,
		IndexContent:         conversionResult.IndexContent,
		DocumentationContent: conversionResult.DocumentationContent,
	}, nil
}

// GenerateFromString generates Bicep extension files from a manifest string
func (g *Generator) GenerateFromString(manifestContent string) (*GenerationResult, error) {
	// Parse the manifest
	provider, err := manifest.ParseManifest(manifestContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate the manifest
	if err := provider.Validate(); err != nil {
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}

	// Convert the manifest to Bicep types
	conversionResult, err := converter.Convert(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to convert manifest: %w", err)
	}

	return &GenerationResult{
		TypesContent:         conversionResult.TypesContent,
		IndexContent:         conversionResult.IndexContent,
		DocumentationContent: conversionResult.DocumentationContent,
	}, nil
}
