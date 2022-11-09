// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package groupswitch

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "switch resourcegroupname",
		Short: "Switch default resource group scope",
		Long: `Switch default resource group scope
	
	Radius workspaces contain a resource group scope, where Radius applications and resources are deployed by default. The switch command changes the default scope of the workspace to the specified resource group name.
	
	Resource groups are used to organize and manage Radius resources. They often contain resources that share a common lifecycle or unit of deployment.
			
	Note that these resource groups are separate from the Azure cloud provider and Azure resource groups configured with the cloud provider.`,
		Example: `rad group switch rgprod -w wsprod`,
		Args:    cobra.MaximumNArgs(1),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)

	return cmd, runner
}

type Runner struct {
	ConfigHolder         *framework.ConfigHolder
	ConnectionFactory    connections.Factory
	Workspace            *workspaces.Workspace
	UCPResourceGroupName string
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	if !workspace.IsEditableWorkspace() {
		// Only workspaces stored in configuration can be modified.
		return workspaces.ErrEditableWorkspaceRequired
	}

	resourceGroup, err := cli.RequireUCPResourceGroup(cmd, args)
	if err != nil {
		return err
	}

	r.UCPResourceGroupName = resourceGroup
	r.Workspace = workspace

	return nil
}

func (r *Runner) Run(ctx context.Context) error {

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	_, err = client.ShowUCPGroup(ctx, "radius", "local", r.UCPResourceGroupName)
	if err != nil {
		return &cli.FriendlyError{Message: fmt.Sprintf("resource group %q does not exist. Run `rad group create` or `rad init` and try again \n", r.UCPResourceGroupName)}
	}

	scope := fmt.Sprintf("/planes/radius/local/resourceGroups/%s", r.UCPResourceGroupName)

	err = cli.EditWorkspaces(ctx, r.ConfigHolder.Config, func(section *cli.WorkspaceSection) error {

		workspace := *r.Workspace
		workspace.Scope = scope
		section.Items[workspace.Name] = workspace

		return nil
	})
	return err

}
