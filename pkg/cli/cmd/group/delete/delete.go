// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package delete

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "delete -g resourcegroupname",
		Short:   "delete RAD resource group",
		Long:    "`Delete radius resource groups. Radius resource group is a radius concept that is used to organize and manage resources. This is NOT the same as Azure resource groups`",
		Example: `rad group delete -g rgprod`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	cmd.Flags().StringP("group", "g", "", "The resource group name")
	cmd.Flags().StringP("workspace", "w", "", "The workspace name")
	cmd.Flags().BoolP("yes", "y", true, "")

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
	Confirmation         bool
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

	kubecontext, ok := workspace.Connection["context"].(string)
	if !ok {
		return fmt.Errorf("cannot delete the resource group. workspace %q has invalid context", workspaceName)
	}

	resourcegroup, err := cli.RequireUCPResourceGroup(cmd)
	if err != nil {
		return err
	}
	if resourcegroup == "" {
		return fmt.Errorf("cannot deletes resource group without specifying its name. use -g to provide the name")
	}

	isRadiusInstalled, err := helm.CheckRadiusInstall(kubecontext)
	if err != nil {
		return err
	}
	if !isRadiusInstalled {
		return fmt.Errorf("radius control plane not installed. run `rad init` or `rad install` and try again")
	}

	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	r.UCPResourceGroupName = resourcegroup
	r.Workspace = workspace
	r.Confirmation = yes

	return nil
}

func (r *Runner) Run(ctx context.Context) error {

	// Prompt user to confirm deletion
	if !r.Confirmation {
		confirmed, err := prompt.ConfirmWithDefault(fmt.Sprintf("Are you sure you want to delete the resource group '%v' and all its contents [y/N]?", r.UCPResourceGroupName), prompt.No)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	fmt.Printf("deleting resource group %q ...\n", r.UCPResourceGroupName)
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
		r.Output.LogInfo("resource group %q and its contents deleted", r.UCPResourceGroupName)
	} else {
		r.Output.LogInfo("resourcegroup %q does not exist or has already been deleted", r.UCPResourceGroupName)
	}
	return nil

}
