// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/group/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	v20220315privatepreview "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create resourcegroupname",
		Short: "Create a new resource group",
		Long: `Create a new resource group

Resource groups are used to organize and manage Radius resources. They often contain resources that share a common lifecycle or unit of deployment.

A Radius application and its resources can span one or more resource groups, and do not have to be in the same resource group as the Radius environment into which it's being deployed into.

Note that these resource groups are separate from the Azure cloud provider and Azure resource groups configured with the cloud provider.
`,
		Example: `rad group create rgprod`,
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
	Output               output.Interface
	Workspace            *workspaces.Workspace
	UCPResourceGroupName string
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

	resourceGroup, err := cli.RequireUCPResourceGroup(cmd, args)
	if err != nil {
		return err
	}

	err = common.ValidateResourceGroupName(resourceGroup)
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

	r.Output.LogInfo("creating resource group %q in workspace %q...\n", r.UCPResourceGroupName, r.Workspace.Name)

	_, err = client.CreateUCPGroup(ctx, "radius", "local", r.UCPResourceGroupName, v20220315privatepreview.ResourceGroupResource{})
	if err != nil {
		return err
	}

	// TODO: we TEMPORARILY create a resource group in the deployments plane because the deployments RP requires it.
	// We'll remove this in the future.
	_, err = client.CreateUCPGroup(ctx, "deployments", "local", r.UCPResourceGroupName, v20220315privatepreview.ResourceGroupResource{})
	if err != nil {
		return err
	}

	r.Output.LogInfo("resource group %q created", r.UCPResourceGroupName)
	return nil

}
