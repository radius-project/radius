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

package list

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad app show` command and runner.
//

// NewCommand creates a new Cobra command for showing Radius application details, which takes in a factory object and
// returns a Cobra command and a Runner object.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show Radius application details",
		Long:  `Show Radius application details. Shows the user's default application (if configured) by default.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Show current application
rad app show

# Show specified application
rad app show my-app

# Show specified application in a specified resource group
rad app show my-app --group my-group
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad app show` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface

	ApplicationName string
	Format          string
}

// NewRunner creates an instance of the runner for the `rad app show` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad app show` command.
//

// Validate checks the workspace, scope, application name and output format from the command line arguments and
// request object, and returns an error if any of these are invalid.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// Allow '--group' to override scope
	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	r.Format = format

	return nil
}

// Run runs the `rad app show` command.
//

// Run() uses the provided context and connection factory to create an applications management client, then attempts to
// show the application with the given name. If the application is not found or has been deleted, an error is
// returned. Otherwise, the application is written to the output in the specified format, and nil is returned.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	application, err := client.ShowApplication(ctx, r.ApplicationName)
	if clients.Is404Error(err) {
		return clierrors.Message("The application %q was not found or has been deleted.", r.ApplicationName)
	} else if err != nil {
		return err
	}

	err = r.Output.WriteFormatted(r.Format, application, objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
