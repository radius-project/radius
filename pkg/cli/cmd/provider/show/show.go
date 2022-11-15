// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/provider/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad provider show` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Show details of a configured cloud provider",
		Long:  "Show details of a configured cloud provider." + common.LongDescriptionBlurb,
		Example: `
# Show cloud providers details for Azure
rad provider show azure
`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad provider show` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace
	Kind              string
}

// NewRunner creates a new instance of the `rad provider show` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad provider show` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// TODO: support fallback workspace
	if !r.Workspace.IsNamedWorkspace() {
		return workspaces.ErrNamedWorkspaceRequired
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	r.Kind = args[0] // Validated by Cobra
	err = common.ValidateCloudProviderName(r.Kind)
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad provider show` command.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Showing cloud provider %q for Radius installation %q...", r.Kind, r.Workspace.FmtConnection())
	client, err := r.ConnectionFactory.CreateCloudProviderManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	providers, err := client.Get(ctx, r.Kind)
	if clients.Is404Error(err) {
		return &cli.FriendlyError{Message: fmt.Sprintf("Cloud provider %q could not be found.", r.Kind)}
	} else if err != nil {
		return err
	}

	err = r.Output.WriteFormatted(r.Format, providers, objectformats.GetCloudProviderTableFormat())
	if err != nil {
		return err
	}

	return nil
}
