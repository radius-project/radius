// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/configFile"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show details of workspace",
		Long:  `Show details of workspace

Workspaces allow you to manage multiple Radius platforms and environments using a local configuration file.

Details include the workspace name, kubectl context, default resource group, and default environment.
`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Show the details of the default workspace
rad workspace show

# Show the details of workspace 'myworkspace'
rad workspace show myworkspace
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	return cmd, runner
}

type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	ConnectionFactory   connections.Factory
	Workspace           *workspaces.Workspace
	ConfigFileInterface configFile.Interface
	Output              output.Interface
	Format              string
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory:   factory.GetConnectionFactory(),
		ConfigHolder:        factory.GetConfigHolder(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		Output:              factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config

	workspace, err := cli.RequireWorkspaceArgs(cmd, config, args)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	if format == "" {
		format = "table"
	}
	r.Format = format

	return nil
}

func (r *Runner) Run(ctx context.Context) error {

	err := r.ConfigFileInterface.ShowWorkspace(r.Output, r.Format, r.Workspace)
	if err != nil {
		return err
	}

	return nil
}
