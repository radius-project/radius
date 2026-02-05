// Package skills provides composable discovery tasks.
package skills

import (
	"fmt"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/output"
)

// GenerateAppDefinitionSkill generates application definitions from discovery results.
type GenerateAppDefinitionSkill struct {
	generator *output.BicepGenerator
}

// NewGenerateAppDefinitionSkill creates a new generate_app_definition skill.
func NewGenerateAppDefinitionSkill() (*GenerateAppDefinitionSkill, error) {
	gen, err := output.NewBicepGenerator()
	if err != nil {
		return nil, fmt.Errorf("creating bicep generator: %w", err)
	}
	return &GenerateAppDefinitionSkill{generator: gen}, nil
}

// Name returns the skill name.
func (s *GenerateAppDefinitionSkill) Name() string {
	return "generate_app_definition"
}

// Description returns a description of the skill.
func (s *GenerateAppDefinitionSkill) Description() string {
	return "Generate Radius application definition (app.bicep) from discovery results"
}

// GenerateAppDefinitionInput contains input for app definition generation.
type GenerateAppDefinitionInput struct {
	// DiscoveryResult contains the discovery results to generate from
	DiscoveryResult *discovery.DiscoveryResult

	// ApplicationName is the name for the Radius application
	ApplicationName string

	// Environment is the target Radius environment
	Environment string

	// OutputPath is the path to write app.bicep (optional, for file generation)
	OutputPath string

	// IncludeComments adds helpful comments to the generated Bicep
	IncludeComments bool

	// IncludeRecipes includes recipe references for infrastructure resources
	IncludeRecipes bool
}

// GenerateAppDefinitionOutput contains the generated application definition.
type GenerateAppDefinitionOutput struct {
	// BicepContent is the generated Bicep file content
	BicepContent string

	// OutputPath is the path where the file was written (if file generation was requested)
	OutputPath string

	// ResourceCount is the number of resources generated
	ResourceCount int

	// Warnings contains any generation warnings
	Warnings []string
}

// Execute generates an application definition.
func (s *GenerateAppDefinitionSkill) Execute(input *GenerateAppDefinitionInput) (*GenerateAppDefinitionOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.DiscoveryResult == nil {
		return nil, fmt.Errorf("discovery result is required")
	}

	opts := output.BicepGenerateOptions{
		ApplicationName: input.ApplicationName,
		Environment:     input.Environment,
		IncludeComments: input.IncludeComments,
		IncludeRecipes:  input.IncludeRecipes,
	}

	out := &GenerateAppDefinitionOutput{}

	// Count resources
	out.ResourceCount = 1 + len(input.DiscoveryResult.Services) + len(input.DiscoveryResult.ResourceTypes)

	// Generate to file if path specified
	if input.OutputPath != "" {
		if err := s.generator.GenerateToFile(input.DiscoveryResult, opts, input.OutputPath); err != nil {
			return nil, fmt.Errorf("generating app.bicep: %w", err)
		}
		out.OutputPath = input.OutputPath
	}

	// Also generate content to string for output
	var buf bytesBuffer
	if err := s.generator.Generate(input.DiscoveryResult, opts, &buf); err != nil {
		return nil, fmt.Errorf("generating bicep content: %w", err)
	}
	out.BicepContent = buf.String()

	// Add warnings for incomplete resources
	for _, svc := range input.DiscoveryResult.Services {
		// Skip bundled services - they won't generate containers
		if svc.IsBundledInto != "" {
			continue
		}
		if len(svc.ExposedPorts) == 0 {
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("service '%s' has no exposed ports; container may not be accessible", svc.Name))
		}
	}

	for _, rt := range input.DiscoveryResult.ResourceTypes {
		if rt.Confidence < 0.7 {
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("resource type mapping for '%s' has low confidence (%.0f%%); review recommended",
					rt.DependencyID, rt.Confidence*100))
		}
	}

	return out, nil
}

// bytesBuffer is a simple bytes.Buffer wrapper for the io.Writer interface.
type bytesBuffer struct {
	data []byte
}

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *bytesBuffer) String() string {
	return string(b.data)
}
