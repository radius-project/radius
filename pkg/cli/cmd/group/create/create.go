// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "create -g resourcegroupname",
		Short:   "create RAD resource group",
		Long:    "`Create radius resource groups. Radius resource group is a radius concept that is used to organize and manage resources. This is NOT the same as Azure resource groups. `",
		Example: `rad group create -g rgprod`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	cmd.Flags().StringP("group", "g", "", "The RAD resource group name")
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

	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	kubecontext, ok := workspace.Connection["context"].(string)
	if !ok {
		return fmt.Errorf("cannot create the resource group. workspace %q has invalid context", workspaceName)
	}

	resourcegroup, err := cli.RequireUCPResourceGroup(cmd)
	if err != nil {
		return err
	}
	if resourcegroup == "" {
		return fmt.Errorf("cannot create resource group without specifying its name. use -g to provide the name")
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

	fmt.Printf("creating resource group %q is workspace %q...\n", r.UCPResourceGroupName, r.Workspace.Name)

	_, err = client.CreateUCPGroup(ctx, "radius", "local", r.UCPResourceGroupName, v20220901privatepreview.ResourceGroupResource{})
	if err != nil {
		return err
	}

	// TODO: we TEMPORARILY create a resource group in the deployments plane because the deployments RP requires it.
	// We'll remove this in the future.
	_, err = client.CreateUCPGroup(ctx, "deployments", "local", r.UCPResourceGroupName, v20220901privatepreview.ResourceGroupResource{})

	if err == nil {
		fmt.Printf("resource group %q created", r.UCPResourceGroupName)
	}

	return err

}
