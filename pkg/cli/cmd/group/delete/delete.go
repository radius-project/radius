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
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete -g resourcegroupname",
		Short: "delete RAD resource group",
		Long:  "`Manage radius resource groups. This is NOT the same as Azure resource groups`",
		Example: `
	radius resource group is a radius concept that is used to organize and manage resources. 
	rad group delete -g rg_prod
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
		return fmt.Errorf("Please use a workspace with valid kubernetes context. %q has invalid context", workspaceName)
	}
	resourcegroup, err := cli.RequireUCPResourceGroup(cmd)
	if err != nil || resourcegroup == "" {
		return fmt.Errorf("please specify a resource group name using -g: %w", err)
	}

	r.UCPResourceGroup = resourcegroup
	r.Workspace = workspace

	return nil
}

func (r *Runner) Run(ctx context.Context) error {

	resourcegroup := r.UCPResourceGroup
	workspaceName := r.Workspace.Name
	contextName, _ := r.Workspace.Connection["context"].(string)

	output.LogInfo("deleting resource group %q in workspace %q ...", resourcegroup, workspaceName)
	//TODO

	output.LogInfo("resource group %q deleted", resourcegroup)
	return nil

}
