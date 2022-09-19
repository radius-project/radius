// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package groupswitch

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "switch -g resourcegroupname",
		Short:   "switch RAD resource group",
		Long:    "`Manage radius resource groups. Radius resource group is a radius concept that is used to organize and manage resources. This is NOT the same as Azure resource groups`",
		Example: `rad group switch -g rgprod -w wsprod`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	cmd.Flags().StringP("group", "g", "", "The resource group name")
	cmd.Flags().StringP("workspace", "w", "", "The workspace name")

	return cmd, runner
}

type Runner struct {
	ConfigHolder         *framework.ConfigHolder
	ConnectionFactory    connections.Factory
	Output               output.Interface
	Workspace            *workspaces.Workspace
	UCPResourceGroupName string
	ResourceType         string
	ResourceName         string
	Format               string
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config
	section, err := cli.ReadWorkspaceSection(config)
	if err != nil {
		return err
	}

	workspaceName, err := cmd.Flags().GetString("workspace")
	if err != nil {
		return err
	}

	if workspaceName == "" {
		section, err := cli.ReadWorkspaceSection(config)
		if err != nil {
			return err
		}
		workspaceName = section.Default
	}
	if workspaceName == "" {
		return fmt.Errorf("no default workspace set. Run`rad workspace switch` or `rad init` and try again")
	}
	workspace, err := section.GetWorkspace(workspaceName)
	if err != nil {
		return err
	}

	resourcegroup, err := cli.RequireUCPResourceGroup(cmd)
	if err != nil {
		return err
	}
	if resourcegroup == "" {
		return fmt.Errorf("cannot switch to resource group without specifying its name. use -g to provide the name")
	}

	kubecontext, ok := workspace.Connection["context"].(string)
	if !ok {
		return fmt.Errorf("cannot switch to the resource group. workspace %q has invalid context", workspaceName)
	}

	isRadiusInstalled, err := helm.CheckRadiusInstall(kubecontext)
	if err != nil {
		return err
	}
	if !isRadiusInstalled {
		return fmt.Errorf("radius control plane not installed. run `rad init` or `rad install` and try again")
	}

	r.UCPResourceGroupName = resourcegroup
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
