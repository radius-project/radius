// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package envswitch

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

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

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		ConnectionFactory: factory.GetConnectionFactory(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config

	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

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
	if cli.Is404ErrorForAzureError(err) {
		return &cli.FriendlyError{Message: fmt.Sprintf("Unable to switch environments as requested environment %s does not exist.\n", r.EnvironmentName)}
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
