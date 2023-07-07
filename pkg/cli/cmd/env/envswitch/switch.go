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

package envswitch

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// NewCommand creates an instance of the command and runner for the `rad env switch` command.
//
// # Function Explanation
//
// NewCommand creates a new cobra command for switching the current environment, which takes in a factory and returns a
// cobra command and a runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "switch [environment]",
		Short:   "Switch the current environment",
		Long:    "Switch the current environment",
		Args:    cobra.MaximumNArgs(1),
		Example: `rad env switch newEnvironment`,
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad env switch` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	Output            output.Interface
	Workspace         *workspaces.Workspace
	ApplicationName   string
	EnvironmentId     resources.ID
	EnvironmentName   string
	Scope             resources.ID
	ConnectionFactory connections.Factory
}

// NewRunner creates a new instance of the `rad env switch` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		ConnectionFactory: factory.GetConnectionFactory(),
	}
}

// Validate runs validation for the `rad env switch` command.
//
// # Function Explanation
//
// Validate checks if the requested environment exists and if it is an editable workspace, then sets the environment
// name, scope, and environment ID for the Runner struct. If the requested environment is already set as the default
// environment, it logs a message. If an error occurs, it is returned.
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

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *r.Workspace)
	if err != nil {
		return err
	}

	// TODO: for right now we assume the environment is in the default resource group.
	r.Scope, err = resources.ParseScope(r.Workspace.Scope)
	if err != nil {
		return err
	}

	r.EnvironmentId = r.Scope.Append(resources.TypeSegment{Type: "Applications.Core/environments", Name: r.EnvironmentName})

	// Keep the logic below here in sync with `rad app switch`
	if strings.EqualFold(r.Workspace.Environment, r.EnvironmentId.String()) {
		r.Output.LogInfo("Default environment is already set to %v", r.EnvironmentName)
		return nil
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}

	// Validate that the environment exists
	_, err = client.GetEnvDetails(cmd.Context(), r.EnvironmentName)
	if clients.Is404Error(err) {
		return clierrors.Message("Unable to switch environments as requested environment %s does not exist.", r.EnvironmentName)
	} else if err != nil {
		return err
	}

	if r.Workspace.Environment == "" {
		r.Output.LogInfo("Switching default environment to %v", r.EnvironmentName)
	} else {
		// Parse the environment ID to get the name
		existing, err := resources.ParseResource(r.Workspace.Environment)
		if err != nil {
			return err
		}

		r.Output.LogInfo("Switching default environment from %v to %v", existing.Name(), r.EnvironmentName)
	}

	return nil
}

// Run runs the `rad env switch` command.
//
// # Function Explanation
//
// Run updates the workspace section of the configuration with the workspace name and environment ID provided. It
// returns an error if the update fails.
func (r *Runner) Run(ctx context.Context) error {
	err := cli.EditWorkspaces(ctx, r.ConfigHolder.Config, func(section *cli.WorkspaceSection) error {
		r.Workspace.Environment = r.EnvironmentId.String()
		section.Items[strings.ToLower(r.Workspace.Name)] = *r.Workspace
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
