/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package discover

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/discovery"
	"github.com/spf13/cobra"
)

const (
	flagProjectPath    = "path"
	flagMinConfidence  = "min-confidence"
	flagIncludeDevDeps = "include-dev"
	flagOutputPath     = "output"
	flagAcceptDefaults = "accept-defaults"
	flagVerbose        = "verbose"
	flagDryRun         = "dry-run"
)

// NewCommand creates an instance of the `rad app discover` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "discover [path]",
		Short: "Analyze codebase and detect infrastructure dependencies",
		Long: `Analyzes a codebase to automatically detect:
- Infrastructure dependencies (databases, caches, message queues, storage)
- Deployable services (based on Dockerfiles, package manifests)
- Team practices (from existing IaC files)

Results are written to ./radius/discovery.md by default.`,
		Args: cobra.MaximumNArgs(1),
		Example: `
# Discover dependencies in current directory
rad app discover

# Discover dependencies in a specific path
rad app discover ./my-project

# Discover with verbose output
rad app discover --verbose

# Dry run (show what would be detected without writing files)
rad app discover --dry-run

# Include development dependencies
rad app discover --include-dev

# Specify custom output path
rad app discover --output ./docs/discovery.md
`,
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().StringP(flagProjectPath, "p", ".", "Path to project directory")
	cmd.Flags().Float64(flagMinConfidence, 0.5, "Minimum confidence threshold (0.0-1.0)")
	cmd.Flags().Bool(flagIncludeDevDeps, false, "Include development dependencies")
	cmd.Flags().StringP(flagOutputPath, "o", "", "Output path for discovery.md (default: ./radius/discovery.md)")
	cmd.Flags().BoolP(flagAcceptDefaults, "y", false, "Accept all defaults without prompting")
	cmd.Flags().BoolP(flagVerbose, "v", false, "Enable verbose output")
	cmd.Flags().Bool(flagDryRun, false, "Show detected dependencies without writing files")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad app discover` command.
type Runner struct {
	Output output.Interface

	ProjectPath    string
	MinConfidence  float64
	IncludeDevDeps bool
	OutputPath     string
	AcceptDefaults bool
	Verbose        bool
	DryRun         bool
}

// NewRunner creates an instance of the runner for the `rad app discover` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Output: factory.GetOutput(),
	}
}

// Validate validates the command line arguments.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Get project path from args or flag
	if len(args) > 0 {
		r.ProjectPath = args[0]
	} else {
		r.ProjectPath, _ = cmd.Flags().GetString(flagProjectPath)
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(r.ProjectPath)
	if err != nil {
		return fmt.Errorf("invalid project path: %w", err)
	}
	r.ProjectPath = absPath

	// Validate project path exists
	if _, err := os.Stat(r.ProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("project path does not exist: %s", r.ProjectPath)
	}

	// Get other flags
	r.MinConfidence, _ = cmd.Flags().GetFloat64(flagMinConfidence)
	r.IncludeDevDeps, _ = cmd.Flags().GetBool(flagIncludeDevDeps)
	r.OutputPath, _ = cmd.Flags().GetString(flagOutputPath)
	r.Verbose, _ = cmd.Flags().GetBool(flagVerbose)
	r.DryRun, _ = cmd.Flags().GetBool(flagDryRun)

	// Set default output path
	if r.OutputPath == "" {
		r.OutputPath = filepath.Join(r.ProjectPath, "radius", "discovery.md")
	} else {
		// Convert to absolute path
		r.OutputPath, _ = filepath.Abs(r.OutputPath)
		// If output path is a directory, append default filename
		if info, err := os.Stat(r.OutputPath); err == nil && info.IsDir() {
			r.OutputPath = filepath.Join(r.OutputPath, "discovery.md")
		}
	}

	// Validate confidence
	if r.MinConfidence < 0 || r.MinConfidence > 1 {
		return fmt.Errorf("min-confidence must be between 0.0 and 1.0")
	}

	return nil
}

// Run executes the `rad app discover` command.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Analyzing project: %s", r.ProjectPath)

	// Create discovery engine
	engine, err := discovery.NewEngine()
	if err != nil {
		return fmt.Errorf("failed to initialize discovery engine: %w", err)
	}

	// Configure discovery options
	opts := discovery.DiscoverOptions{
		ProjectPath:    r.ProjectPath,
		MinConfidence:  r.MinConfidence,
		IncludeDevDeps: r.IncludeDevDeps,
		Verbose:        r.Verbose,
	}

	// Set output path unless dry run
	if !r.DryRun {
		opts.OutputPath = r.OutputPath
	}

	// Run discovery
	result, err := engine.DiscoverAndWrite(ctx, opts)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	// Display summary
	r.displaySummary(result)

	// Show warnings
	for _, w := range result.Warnings {
		r.Output.LogInfo("  [%s] %s", w.Level, w.Message)
	}

	if !r.DryRun {
		r.Output.LogInfo("")
		r.Output.LogInfo("Discovery results written to: %s", r.OutputPath)
		r.Output.LogInfo("")
		r.Output.LogInfo("Next steps:")
		r.Output.LogInfo("  1. Review the discovery results")
		r.Output.LogInfo("  2. Run 'rad app generate' to create app.bicep")
	}

	return nil
}

func (r *Runner) displaySummary(result *discovery.DiscoveryResult) {
	r.Output.LogInfo("")
	r.Output.LogInfo("=== Discovery Summary ===")
	r.Output.LogInfo("")

	// Services
	r.Output.LogInfo("Services detected: %d", len(result.Services))
	for _, svc := range result.Services {
		lang := string(svc.Language)
		if svc.Framework != "" {
			lang = fmt.Sprintf("%s (%s)", lang, svc.Framework)
		}
		r.Output.LogInfo("  - %s [%s] (confidence: %.0f%%)", svc.Name, lang, svc.Confidence*100)
	}

	r.Output.LogInfo("")

	// Dependencies
	r.Output.LogInfo("Dependencies detected: %d", len(result.Dependencies))
	for _, dep := range result.Dependencies {
		r.Output.LogInfo("  - %s: %s v%s (confidence: %.0f%%)", dep.Type, dep.Library, dep.Version, dep.Confidence*100)
	}

	r.Output.LogInfo("")

	// Resource Types
	if len(result.ResourceTypes) > 0 {
		r.Output.LogInfo("Resource Type mappings: %d", len(result.ResourceTypes))
		for _, rt := range result.ResourceTypes {
			r.Output.LogInfo("  - %s → %s", rt.DependencyID, rt.ResourceType.Name)
		}
		r.Output.LogInfo("")
	}

	// Overall confidence
	r.Output.LogInfo("Overall confidence: %.0f%%", result.Confidence*100)
	r.Output.LogInfo("")
}
