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
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create -g resourcegroupname",
		Short: "create RAD resource group",
		Long:  "`Manage radius resource groups. This is NOT the same as Azure resource groups`",
		Example: `
	radius resource group is a radius concept that is used to organize and manage resources. 
	# show details of a specified resource in the default environment

	rad group create -g rg_prod
	`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().StringP("group", "g", "", "The resource group name")
	return cmd, runner
}

type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	UCPResourceGroup  string
	ResourceType      string
	ResourceName      string
	Format            string
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
	workspaceName := section.Default
	workspace, err := section.GetWorkspace(workspaceName)
	if err != nil {
		return err
	}
	_, ok := workspace.Connection["context"].(string)
	if !ok {
		return fmt.Errorf("cannot create the resource group. workspace %q has invalid context", workspaceName)
	}
	resourcegroup, err := cli.RequireUCPResourceGroup(cmd)
	if err != nil {
		return fmt.Errorf("failed to create resource group: %w", err)
	}
	if resourcegroup == "" {
		return fmt.Errorf("cannot create resource group without specifying its name. use -g to provide the name")
	}

	r.UCPResourceGroup = resourcegroup
	r.Workspace = workspace

	return nil
}

func (r *Runner) Run(ctx context.Context) error {

	resourcegroup := r.UCPResourceGroup
	workspaceName := r.Workspace.Name
	contextName, _ := r.Workspace.Connection["context"].(string)

	output.LogInfo("creating resource group %q in workspace %q ...", resourcegroup, workspaceName)
	id, err := setup.CreateWorkspaceResourceGroup(ctx, &workspaces.KubernetesConnection{Context: contextName}, resourcegroup)
	if err != nil {
		return err
	}

	output.LogInfo("resource group %q created", id)
	return nil

}
