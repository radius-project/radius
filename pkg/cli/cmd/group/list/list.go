// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package list

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "list RAD resource group",
		Long:    "`List radius resource groups. Radius resource group is a radius concept that is used to organize and manage resources. This is NOT the same as Azure resource groups`",
		Example: `rad group list`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

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
	format, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	if format == "" {
		format = "table"
	}
	r.Format = format

	kubecontext, ok := workspace.Connection["context"].(string)
	if !ok {
		return fmt.Errorf("cannot delete the resource group. workspace %q has invalid context", workspaceName)
	}

	isRadiusInstalled, err := helm.CheckRadiusInstall(kubecontext)
	if err != nil {
		return err
	}
	if !isRadiusInstalled {
		return fmt.Errorf("radius control plane not installed. run `rad init` or `rad install` and try again")
	}

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

	fmt.Print(resourceGroupDetails)

	err = r.Output.WriteFormatted(r.Format, resourceGroupDetails, objectformats.GetResourceGroupTableFormat())

	if err != nil {
		return err
	}
	return err

}
