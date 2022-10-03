// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package show

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show resourcegroupname",
		Short: "Show the details of a resource group",
		Long: `Show the details of a resource group

Resource groups are used to organize and manage Radius resources. They often contain resources that share a common lifecycle or unit of deployment.

A Radius application and its resources can span one or more resource groups, and do not have to be in the same resource group as the Radius environment into which it's being deployed into.

Note that these resource groups are separate from the Azure cloud provider and Azure resource groups configured with the cloud provider.
`,
		Example: `rad group show rgprod`,
		Args:    cobra.MaximumNArgs(1),
		RunE:    framework.RunCommand(runner),
	}

	cmd.Flags().StringP("group", "g", "", "The resource group name")
	cmd.Flags().StringP("workspace", "w", "", "The workspace name")
	cmd.Flags().StringP("output", "o", "", "The output format")

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
	workspace, err := cli.RequireWorkspaceArgs(cmd, config, args)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	if format == "" {
		format = "table"
	}
	resourcegroup, err := cli.RequireUCPResourceGroup(cmd, args)
	if err != nil {
		return err
	}

	r.Format = format
	r.UCPResourceGroupName = resourcegroup
	r.Workspace = workspace

	return nil
}

func (r *Runner) Run(ctx context.Context) error {

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resourceGroup, err := client.ShowUCPGroup(ctx, "radius", "local", r.UCPResourceGroupName)
	if err != nil {
		return err
	}

	err = r.Output.WriteFormatted(r.Format, resourceGroup, objectformats.GetResourceGroupTableFormat())

	if err != nil {
		return err
	}
	return err

}
