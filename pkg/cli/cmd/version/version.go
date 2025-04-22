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

// Add this new method
func (r *Runner) writeCliVersionOnly(format string) error {
	cliVersion := struct {
		Release string `json:"release"`
		Version string `json:"version"`
		Bicep   string `json:"bicep"`
		Commit  string `json:"commit"`
	}{
		version.Release(),
		version.Version(),
		bicep.Version(),
		version.Commit(),
	}

	// No header when only showing CLI info
	return r.Output.WriteFormatted(format, cliVersion, output.FormatterOptions{Columns: []output.Column{
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
	}})
}

// writeVersionInfo displays both CLI and Control Plane version information
func (r *Runner) writeVersionInfo(format string) error {
	// Display CLI version information
	cliVersion := struct {
		Release string `json:"release"`
		Version string `json:"version"`
		Bicep   string `json:"bicep"`
		Commit  string `json:"commit"`
	}{
		version.Release(),
		version.Version(),
		bicep.Version(),
		version.Commit(),
	}

	r.Output.LogInfo("CLI Version Information:")
	err := r.Output.WriteFormatted(format, cliVersion, output.FormatterOptions{Columns: []output.Column{
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
	}})

	if err != nil {
		return err
	}

	// Then get and display Control Plane version
	controlPlaneVersion := "Not installed"
	daprVersion := "Not installed"

	state, err := r.Helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		r.Output.LogInfo("Failed to check Radius control plane: %v", err)
	} else if state.RadiusInstalled {
		controlPlaneVersion = state.RadiusVersion
		daprVersion = state.DaprVersion
	}

	cpInfo := struct {
		Version     string `json:"version"`
		DaprVersion string `json:"daprVersion"`
	}{
		Version:     controlPlaneVersion,
		DaprVersion: daprVersion,
	}

	// Add Contour version if installed

	r.Output.LogInfo("\nControl Plane Information:")
	return r.Output.WriteFormatted(format, cpInfo, output.FormatterOptions{Columns: []output.Column{
		{
			Heading:  "VERSION",
			JSONPath: "{ .Version }",
		},
		{
			Heading:  "DAPR VERSION",
			JSONPath: "{ .DaprVersion }",
		},
	}})
}
