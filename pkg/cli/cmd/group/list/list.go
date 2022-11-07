// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package list

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
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
		Use:   "list",
		Short: "List resource groups within current/specified workspace",
		Long: `List resource groups within current/specified workspace
	
	Resource groups are used to organize and manage Radius resources. They often contain resources that share a common lifecycle or unit of deployment.
			
	A Radius application and its resources can span one or more resource groups, and do not have to be in the same resource group as the Radius environment into which it's being deployed into.
			
	Note that these resource groups are separate from the Azure cloud provider and Azure resource groups configured with the cloud provider.`,
		Example: `rad group list`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
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
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	// TODO: support fallback workspace
	if !workspace.IsNamedWorkspace() {
		return workspaces.ErrNamedWorkspaceRequired
	}

	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	if format == "" {
		format = "table"
	}
	r.Format = format
	r.Workspace = workspace

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resourceGroupDetails, err := client.ListUCPGroup(ctx, "radius", "local")
	if err != nil {
		return err
	}

	err = r.Output.WriteFormatted(r.Format, resourceGroupDetails, objectformats.GetResourceGroupTableFormat())

	if err != nil {
		return err
	}
	return err
}
