package skills

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/analyzers"
	"github.com/radius-project/radius/pkg/discovery/catalog"
)

// DiscoverDependenciesSkill detects infrastructure dependencies in a codebase.
type DiscoverDependenciesSkill struct {
	registry *analyzers.Registry
	catalog  *catalog.LibraryCatalog
}

// NewDiscoverDependenciesSkill creates the discover_dependencies skill.
func NewDiscoverDependenciesSkill(registry *analyzers.Registry, cat *catalog.LibraryCatalog) *DiscoverDependenciesSkill {
	return &DiscoverDependenciesSkill{
		registry: registry,
		catalog:  cat,
	}
}

// Name returns the skill identifier.
func (s *DiscoverDependenciesSkill) Name() string {
	return "discover_dependencies"
}

// Description returns a human-readable description.
func (s *DiscoverDependenciesSkill) Description() string {
	return "Analyzes a codebase to detect infrastructure dependencies (databases, caches, message queues, storage) from package manifests and import statements."
}

// InputSchema returns the JSON Schema for input parameters.
func (s *DiscoverDependenciesSkill) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"projectPath": map[string]interface{}{
				"type":        "string",
				"description": "Path to the project directory to analyze",
			},
			"minConfidence": map[string]interface{}{
				"type":        "number",
				"description": "Minimum confidence threshold (0.0-1.0)",
				"default":     0.5,
			},
			"includeDevDeps": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to include development dependencies",
				"default":     false,
			},
		},
		"required": []string{"projectPath"},
	}
}

// Execute runs the skill.
func (s *DiscoverDependenciesSkill) Execute(ctx context.Context, input SkillInput) (SkillOutput, error) {
	if input.ProjectPath == "" {
		return NewErrorOutput(fmt.Errorf("projectPath is required")), nil
	}

	opts := analyzers.DefaultAnalyzeOptions()

	// Apply parameters from input
	if minConf, ok := input.Parameters["minConfidence"].(float64); ok {
		opts.MinConfidence = minConf
	}
	if includeDevDeps, ok := input.Parameters["includeDevDeps"].(bool); ok {
		opts.IncludeDevDeps = includeDevDeps
	}

	// Run analyzers
	results, err := s.registry.DetectAndAnalyze(ctx, input.ProjectPath, opts)
	if err != nil {
		return NewErrorOutput(err), nil
	}

	// Aggregate dependencies from all analyzers
	var allDependencies []discovery.DetectedDependency
	var warnings []string

	for _, result := range results {
		allDependencies = append(allDependencies, result.Dependencies...)
		for _, w := range result.Warnings {
			warnings = append(warnings, w.Message)
		}
	}

	output := NewSuccessOutput(map[string]interface{}{
		"dependencies": allDependencies,
		"count":        len(allDependencies),
	})
	output.Warnings = warnings

	return output, nil
}

func init() {
	if analyzers.DefaultRegistry != nil && catalog.DefaultCatalog != nil {
		skill := NewDiscoverDependenciesSkill(analyzers.DefaultRegistry, catalog.DefaultCatalog)
		_ = Register(skill)
	}
}

// NewDiscoverDependenciesSkillWithDefaults creates a discover_dependencies skill with default registries.
// Returns nil if default registries are not initialized.
func NewDiscoverDependenciesSkillWithDefaults() *DiscoverDependenciesSkill {
	registry := analyzers.DefaultRegistry
	libCatalog := catalog.DefaultCatalog

	if registry == nil {
		registry = analyzers.NewRegistry()
	}

	return NewDiscoverDependenciesSkill(registry, libCatalog)
}
