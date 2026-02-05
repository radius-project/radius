// Package analyzers provides language-specific analyzers for detecting
// infrastructure dependencies in codebases.
package analyzers

import (
	"context"

	"github.com/radius-project/radius/pkg/discovery/dtypes"
)

// Analyzer is the interface for language-specific dependency detection.
type Analyzer interface {
	// Name returns the analyzer's identifier.
	Name() string

	// Languages returns the languages this analyzer supports.
	Languages() []dtypes.Language

	// CanAnalyze returns true if this analyzer can process the given path.
	// This is a quick check (e.g., look for package.json, go.mod).
	CanAnalyze(ctx context.Context, projectPath string) (bool, error)

	// Analyze scans the project and returns detected dependencies.
	Analyze(ctx context.Context, projectPath string, opts AnalyzeOptions) (*AnalyzeResult, error)
}

// AnalyzeOptions configures analyzer behavior.
type AnalyzeOptions struct {
	// MinConfidence is the minimum confidence threshold for dependencies (0.0-1.0).
	MinConfidence float64

	// IncludeDevDeps indicates whether to include dev/test dependencies.
	IncludeDevDeps bool

	// MaxDepth limits directory traversal depth (0 = unlimited).
	MaxDepth int
}

// DefaultAnalyzeOptions returns sensible defaults.
func DefaultAnalyzeOptions() AnalyzeOptions {
	return AnalyzeOptions{
		MinConfidence:  0.5,
		IncludeDevDeps: false,
		MaxDepth:       0,
	}
}

// AnalyzeResult contains the output of an analyzer.
type AnalyzeResult struct {
	// Language detected for this project.
	Language dtypes.Language

	// Services detected in this project.
	Services []dtypes.Service

	// Dependencies detected in this project.
	Dependencies []dtypes.DetectedDependency

	// Warnings encountered during analysis.
	Warnings []dtypes.DiscoveryWarning
}

// NewAnalyzeResult creates an empty result for the given language.
func NewAnalyzeResult(lang dtypes.Language) *AnalyzeResult {
	return &AnalyzeResult{
		Language:     lang,
		Services:     make([]dtypes.Service, 0),
		Dependencies: make([]dtypes.DetectedDependency, 0),
		Warnings:     make([]dtypes.DiscoveryWarning, 0),
	}
}

// AddDependency appends a dependency to the result.
func (r *AnalyzeResult) AddDependency(dep dtypes.DetectedDependency) {
	r.Dependencies = append(r.Dependencies, dep)
}

// AddService appends a service to the result.
func (r *AnalyzeResult) AddService(svc dtypes.Service) {
	r.Services = append(r.Services, svc)
}

// AddWarning appends a warning to the result.
func (r *AnalyzeResult) AddWarning(warning dtypes.DiscoveryWarning) {
	r.Warnings = append(r.Warnings, warning)
}
