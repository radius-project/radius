// Package app provides CLI commands for application management.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/discovery/output/templates"
	"github.com/radius-project/radius/pkg/discovery/practices"
	"github.com/radius-project/radius/pkg/discovery/scaffold"
)

const (
	flagScaffoldName        = "name"
	flagScaffoldPath        = "path"
	flagScaffoldEnv         = "environment"
	flagScaffoldTemplate    = "template"
	flagScaffoldAddDep      = "add-dependency"
	flagScaffoldInteractive = "interactive"
	flagScaffoldForce       = "force"
)

// NewScaffoldCommand creates a new scaffold command.
func NewScaffoldCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewScaffoldRunner(factory)

	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Scaffold a new Radius application",
		Long: `Scaffolds a new Radius application with the necessary files and structure.

This command creates a new application directory with:
- app.bicep: The Bicep application definition
- radius/: Directory for Radius-specific configuration
- Dockerfile (optional): If container deployment is selected

Examples:
  # Scaffold with interactive prompts
  rad app scaffold --name myapp

  # Scaffold with specific dependencies
  rad app scaffold --name myapp --add-dependency postgres --add-dependency redis

  # Scaffold from a template
  rad app scaffold --name myapp --template web-api

  # Scaffold in a specific directory
  rad app scaffold --name myapp --path ./projects/myapp
`,
		Example: `  rad app scaffold --name myapp
  rad app scaffold --name myapp --add-dependency postgres
  rad app scaffold --name myapp --template web-api --environment staging`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	// Required flags
	cmd.Flags().StringP(flagScaffoldName, "n", "", "Name of the application (required)")
	_ = cmd.MarkFlagRequired(flagScaffoldName)

	// Optional flags
	cmd.Flags().StringP(flagScaffoldPath, "p", ".", "Path where to create the application")
	cmd.Flags().StringP(flagScaffoldEnv, "e", "default", "Target environment name")
	cmd.Flags().StringP(flagScaffoldTemplate, "t", "", "Application template to use (e.g., web-api, worker, frontend)")
	cmd.Flags().StringArrayP(flagScaffoldAddDep, "d", nil, "Add infrastructure dependency (can be specified multiple times)")
	cmd.Flags().BoolP(flagScaffoldInteractive, "i", true, "Run in interactive mode")
	cmd.Flags().BoolP(flagScaffoldForce, "f", false, "Force overwrite if directory exists")

	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// ScaffoldRunner implements the scaffold command.
type ScaffoldRunner struct {
	factory      framework.Factory
	Output       output.Interface
	Name         string
	Path         string
	Environment  string
	Template     string
	Dependencies []string
	Interactive  bool
	Force        bool
}

// NewScaffoldRunner creates a new ScaffoldRunner.
func NewScaffoldRunner(factory framework.Factory) *ScaffoldRunner {
	return &ScaffoldRunner{
		factory: factory,
	}
}

// Validate validates the command arguments.
func (r *ScaffoldRunner) Validate(cmd *cobra.Command, args []string) error {
	r.Name, _ = cmd.Flags().GetString(flagScaffoldName)
	r.Path, _ = cmd.Flags().GetString(flagScaffoldPath)
	r.Environment, _ = cmd.Flags().GetString(flagScaffoldEnv)
	r.Template, _ = cmd.Flags().GetString(flagScaffoldTemplate)
	r.Dependencies, _ = cmd.Flags().GetStringArray(flagScaffoldAddDep)
	r.Interactive, _ = cmd.Flags().GetBool(flagScaffoldInteractive)
	r.Force, _ = cmd.Flags().GetBool(flagScaffoldForce)

	if r.Name == "" {
		return fmt.Errorf("application name is required")
	}

	// Validate name (alphanumeric, hyphens, underscores only)
	if !isValidAppName(r.Name) {
		return fmt.Errorf("invalid application name: must contain only alphanumeric characters, hyphens, or underscores")
	}

	return nil
}

// Run executes the scaffold command.
func (r *ScaffoldRunner) Run(ctx context.Context) error {
	r.Output = r.factory.GetOutput()

	// Determine output path
	outputPath := filepath.Join(r.Path, r.Name)
	if r.Path != "." {
		outputPath = r.Path
	}

	// Check if directory exists
	if _, err := os.Stat(outputPath); err == nil && !r.Force {
		return fmt.Errorf("directory %q already exists. Use --force to overwrite", outputPath)
	}

	// If interactive mode, prompt for additional options
	if r.Interactive && len(r.Dependencies) == 0 && r.Template == "" {
		deps, err := r.promptForDependencies()
		if err != nil {
			return err
		}
		r.Dependencies = deps
	}

	// Create scaffolder
	scaffolder := scaffold.NewScaffolder(scaffold.ScaffoldOptions{
		Name:         r.Name,
		OutputPath:   outputPath,
		Environment:  r.Environment,
		Template:     r.Template,
		Dependencies: r.Dependencies,
	})

	// Load team practices if available
	practicesPath := filepath.Join(r.Path, ".radius", "team-practices.yaml")
	if teamPractices, err := practices.LoadConfigFromFile(practicesPath); err == nil {
		scaffolder.WithPractices(&teamPractices.Practices)
	}

	r.Output.LogInfo("Creating application scaffold...")
	r.Output.LogInfo("  Name: %s", r.Name)
	r.Output.LogInfo("  Path: %s", outputPath)
	r.Output.LogInfo("  Environment: %s", r.Environment)

	if len(r.Dependencies) > 0 {
		r.Output.LogInfo("  Dependencies: %s", strings.Join(r.Dependencies, ", "))
	}

	// Execute scaffolding
	result, err := scaffolder.Scaffold(ctx)
	if err != nil {
		return fmt.Errorf("scaffolding failed: %w", err)
	}

	// Output results
	r.Output.LogInfo("")
	r.Output.LogInfo("✓ Application scaffolded successfully!")
	r.Output.LogInfo("")
	r.Output.LogInfo("Created files:")
	for _, file := range result.CreatedFiles {
		r.Output.LogInfo("  - %s", file)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	r.Output.LogInfo("  1. cd %s", outputPath)
	r.Output.LogInfo("  2. Review and customize app.bicep")
	r.Output.LogInfo("  3. rad deploy app.bicep")

	return nil
}

func (r *ScaffoldRunner) promptForDependencies() ([]string, error) {
	// For non-interactive mode or testing, return empty
	if r.Output == nil {
		return nil, nil
	}

	// Get available dependency types
	availableDeps := templates.GetAvailableDependencyTypes()

	r.Output.LogInfo("Select infrastructure dependencies for your application:")
	r.Output.LogInfo("")

	for i, dep := range availableDeps {
		r.Output.LogInfo("  [%d] %s - %s", i+1, dep.Name, dep.Description)
	}
	r.Output.LogInfo("  [0] None - Skip adding dependencies")
	r.Output.LogInfo("")

	return nil, nil
}

func isValidAppName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for i, c := range name {
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9' && i > 0) ||
			c == '-' || c == '_') {
			return false
		}
	}
	return true
}
