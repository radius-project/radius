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

package appswitch

import (
	"context"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad app switch` command.
//

// NewCommand creates a new cobra command for switching the default Radius Application, which takes in a factory and
// returns a cobra command and a runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	cmd := &cobra.Command{
		Use:     "switch",
		Short:   "Switch the default Radius Application",
		Long:    "Switches the default Radius Application",
		Args:    cobra.MaximumNArgs(1),
		Example: `rad app switch newApplication`,
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad app switch` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	Output            output.Interface
	Workspace         *workspaces.Workspace
	ApplicationName   string
	ConnectionFactory connections.Factory
}

// NewRunner creates a new instance of the `rad app switch` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		ConnectionFactory: factory.GetConnectionFactory(),
	}
}

// Validate runs validation for the `rad app switch` command.
//

// Validate checks if the workspace is editable, reads the application name from the command line arguments, checks
// if the application exists. It returns an error if the workspace is not editable, if the application name is not provided,
// or if the application does not exist.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	if !r.Workspace.IsEditableWorkspace() {
		// Only workspaces stored in configuration can be modified.
		return workspaces.ErrEditableWorkspaceRequired
	}

	r.ApplicationName, err = cli.ReadApplicationNameArgs(cmd, args)
	if err != nil {
		return err
	}

	// Keep the logic below here in sync with `rad env switch``
	if strings.EqualFold(r.Workspace.DefaultApplication, r.ApplicationName) {
		r.Output.LogInfo("Default application is already set to %v", r.ApplicationName)
		return nil
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}

	// Validate that the application exists
	_, err = client.ShowApplication(cmd.Context(), r.ApplicationName)
	if clients.Is404Error(err) {
		return clierrors.Message("Unable to switch applications as the requested application %s does not exist.", r.ApplicationName)
	} else if err != nil {
		return err
	}

	if workspace.DefaultApplication == "" {
		r.Output.LogInfo("Switching default application to %v", r.ApplicationName)
	} else {
		r.Output.LogInfo("Switching default application from %v to %v", workspace.DefaultApplication, r.ApplicationName)
	}

	return nil
}

// Run runs the `rad app switch` command.
//

// The function Run takes in a context and updates the configuration of the workspace with the given application name,
// and returns an error if any.
func (r *Runner) Run(ctx context.Context) error {
	err := cli.EditWorkspaces(ctx, r.ConfigHolder.Config, func(section *cli.WorkspaceSection) error {
		r.Workspace.DefaultApplication = r.ApplicationName
		section.Items[strings.ToLower(r.Workspace.Name)] = *r.Workspace
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
