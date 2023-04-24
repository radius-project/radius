// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad workspace show` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show local workspace",
		Long:  `Show local workspace`,
		Example: `# Show current workspace
rad workspace show

# Show named workspace
rad workspace show my-workspace`,
		Args: cobra.RangeArgs(0, 1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad workspace show` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Format       string
	Workspace    *workspaces.Workspace
}

// NewRunner creates a new instance of the `rad workspace show` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

// Validate runs validation for the `rad workspace show` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspaceArgs(cmd, r.ConfigHolder.Config, args)
	if err != nil {
		return err
	}

	if !workspace.IsNamedWorkspace() {
		return workspaces.ErrEditableWorkspaceRequired
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	r.Workspace = workspace
	r.Format = format

	return nil
}

// Run runs the `rad workspace show` command.
func (r *Runner) Run(ctx context.Context) error {
	err := r.Output.WriteFormatted(r.Format, r.Workspace, objectformats.GetWorkspaceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
