package version

import (
	"context"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/version"
	"github.com/spf13/cobra"
)

type CLIVersionInfo struct {
	Release string `json:"release"`
	Version string `json:"version"`
	Bicep   string `json:"bicep"`
	Commit  string `json:"commit"`
}

type ControlPlaneVersionInfo struct {
	Version string `json:"version"`
	Status  string `json:"status"`
}

// getCliVersionInfo returns the CLI version information
func getCliVersionInfo() CLIVersionInfo {
	return CLIVersionInfo{
		Release: version.Release(),
		Version: version.Version(),
		Bicep:   bicep.Version(),
		Commit:  version.Commit(),
	}
}

// getControlPlaneVersionInfo retrieves the Control Plane version information
func (r *Runner) getControlPlaneVersionInfo() ControlPlaneVersionInfo {
	cpInfo := ControlPlaneVersionInfo{
		Version: "Not installed",
		Status:  "Not connected",
	}

	state, err := r.Helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		// Keep the default "Not connected" status
		return cpInfo
	}

	// Connection successful, update status
	cpInfo.Status = "Not installed"

	if state.RadiusInstalled {
		cpInfo.Status = "Installed"
		cpInfo.Version = state.RadiusVersion
	}

	return cpInfo
}

// Update the NewCommand function
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Prints the versions of the rad CLI and the Control Plane",
		Long: `Display version information for the rad CLI and the Control Plane.
By default this shows all available version information.`,
		Example: `# Show all version information
rad version

# Show only the CLI version
rad version --cli`,
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().Bool("cli", false, "Use this flag to only show the rad CLI version")
	return cmd, runner
}

// Runner is the Runner implementation for the version command
type Runner struct {
	Helm        helm.Interface
	Output      output.Interface
	KubeContext string
	Format      string
	CLIOnly     bool
}

// NewRunner creates a new Runner instance
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Output: factory.GetOutput(),
		Helm:   factory.GetHelmInterface(),
	}
}

// Validate validates the command arguments
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	if format == "" {
		format = "table"
	}
	r.Format = format

	cliOnly, err := cmd.Flags().GetBool("cli")
	if err != nil {
		return err
	}
	r.CLIOnly = cliOnly

	return nil
}

// Run executes the version command
func (r *Runner) Run(ctx context.Context) error {
	if r.CLIOnly {
		return r.writeCliVersionOnly(r.Format)
	}

	return r.writeVersionInfo(r.Format)
}

// writeCliVersionOnly displays only CLI version information
func (r *Runner) writeCliVersionOnly(format string) error {
	// No header when only showing CLI info
	return r.Output.WriteFormatted(format, getCliVersionInfo(), getCliVersionFormatterOptions())
}

// writeVersionInfo displays both CLI and Control Plane version information
func (r *Runner) writeVersionInfo(format string) error {
	// Display CLI version information
	cliVersion := getCliVersionInfo()

	// Only show headers for human-readable formats
	if format != "json" && format != "yaml" {
		r.Output.LogInfo("CLI Version Information:")
	}

	err := r.Output.WriteFormatted(format, cliVersion, getCliVersionFormatterOptions())
	if err != nil {
		return err
	}

	// Get control plane info (handles errors internally)
	cpInfo := r.getControlPlaneVersionInfo()

	// Only show headers for human-readable formats
	if format != "json" && format != "yaml" {
		r.Output.LogInfo("\nControl Plane Information:")
	}

	return r.Output.WriteFormatted(format, cpInfo, getControlPlaneFormatterOptions())
}

// getCliVersionFormatterOptions returns formatter options for CLI version information
func getCliVersionFormatterOptions() output.FormatterOptions {
	return output.FormatterOptions{Columns: []output.Column{
		{
			Heading:  "RELEASE",
			JSONPath: "{ .Release }",
		},
		{
			Heading:  "VERSION",
			JSONPath: "{ .Version }",
		},
		{
			Heading:  "BICEP",
			JSONPath: "{ .Bicep }",
		},
		{
			Heading:  "COMMIT",
			JSONPath: "{ .Commit }",
		},
	}}
}

// getControlPlaneFormatterOptions returns formatter options for Control Plane version information
func getControlPlaneFormatterOptions() output.FormatterOptions {
	return output.FormatterOptions{Columns: []output.Column{
		{
			Heading:  "STATUS",
			JSONPath: "{ .Status }",
		},
		{
			Heading:  "VERSION",
			JSONPath: "{ .Version }",
		},
	}}
}
