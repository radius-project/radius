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

// NewCommand creates an instance of the command and runner for the `rad group delete` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete resourcegroupname",
		Short: "Delete a resource group",
		Long: `Delete a resource group. 
		
		Delete a resource group if it is empty. If not empty, delete the contents and try again`,
		Example: `rad group delete rgprod`,
		Args:    cobra.MaximumNArgs(1),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad group delete` command.
type Runner struct {
	ConfigHolder         *framework.ConfigHolder
	ConnectionFactory    connections.Factory
	Output               output.Interface
	InputPrompter        prompt.InputPrompter
	Workspace            *workspaces.Workspace
	UCPResourceGroupName string
	Confirmation         bool
}

// NewRunner creates a new instance of the `rad group delete` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		InputPrompter:     factory.GetInputPrompter(),
	}
}

// Validate runs validation for the `rad group delete` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	resourceGroup, err := cli.RequireUCPResourceGroup(cmd, args)
	if err != nil {
		return err
	}

	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	r.UCPResourceGroupName = resourceGroup
	r.Workspace = workspace
	r.Confirmation = yes

	return nil
}

// Run runs the `rad group delete` command.
func (r *Runner) Run(ctx context.Context) error {

	// Prompt user to confirm deletion
	if !r.Confirmation {
		confirmed, err := prompt.YesOrNoPrompt(
			fmt.Sprintf("Are you sure you want to delete the resource group '%v'? A resource group can be deleted only when empty", r.UCPResourceGroupName),
			r.InputPrompter)
		if err != nil {
			return err
		}

		if !confirmed {
			r.Output.LogInfo("resource group %q NOT deleted", r.UCPResourceGroupName)
			return nil
		}
	}

	r.Output.LogInfo("deleting resource group %q ...\n", r.UCPResourceGroupName)
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	_, err = client.DeleteUCPGroup(ctx, "radius", "local", r.UCPResourceGroupName)
	if err != nil {
		return err
	}

	deleted, err := client.DeleteUCPGroup(ctx, "deployments", "local", r.UCPResourceGroupName)
	if err != nil {
		return err
	}

	if deleted {
		r.Output.LogInfo("resource group %q deleted", r.UCPResourceGroupName)
	} else {
		r.Output.LogInfo("resource group %q does not exist or has already been deleted", r.UCPResourceGroupName)
	}
	return nil
}
