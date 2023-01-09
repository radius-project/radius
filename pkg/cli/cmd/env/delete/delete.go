// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package delete

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

const (
	deleteConfirmation = "Are you sure you want to delete environment '%v'?"
)

// NewCommand creates an instance of the command and runner for the `rad env delete` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete environment",
		Long:  `Delete environment. Deletes the user's default environment by default.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Delete current enviroment
rad env delete

# Delete current enviroment and bypass confirmation prompt
rad env delete --yes

# Delete specified enviroment
rad env delete my-env

# Delete specified enviroment in a specified resource group
rad env delete my-env --group my-env
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad env delete` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface
	InputPrompter     prompt.Interface

	Confirm         bool
	EnvironmentName string
	Format          string
}

// NewRunner creates a new instance of the `rad env delete` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		InputPrompter:     factory.GetPrompter(),
	}
}

// Validate runs validation for the `rad env delete` command.
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

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	r.Confirm, err = cmd.Flags().GetBool("yes")
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

// Run runs the `rad env run` command.
func (r *Runner) Run(ctx context.Context) error {
	// Prompt user to confirm deletion
	if !r.Confirm {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(deleteConfirmation, r.EnvironmentName), "no", r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	deleted, err := client.DeleteEnv(ctx, r.EnvironmentName)
	if err != nil {
		return err
	}

	if deleted {
		r.Output.LogInfo("Environment deleted")
	} else {
		r.Output.LogInfo("Environment '%s' does not exist or has already been deleted.", r.EnvironmentName)
	}

	return nil
}
