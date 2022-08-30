// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package cmd

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// envShowCmd command returns properties of an environment
var groupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "create RAD resource group",
	Long:  "creates a radius resource group. Note: This is not the same as creating an azure resource group",
	RunE:  createGroup,
}

func init() {
	groupCmd.AddCommand(groupCreateCmd)
	groupCreateCmd.PersistentFlags().StringP("group", "g", "", "RAD resource group name")

}
func createGroup(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	section, err := cli.ReadWorkspaceSection(config)
	if err != nil {
		return err
	}
	workspaceName := section.Default
	workspace, err := section.GetWorkspace(workspaceName)
	if err != nil {
		return err
	}
	contextName, ok := workspace.Connection["context"].(string)
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

	output.LogInfo("creating resource group %q in workspace %q ...", resourcegroup, workspaceName)
	id, err := setup.CreateWorkspaceResourceGroup(cmd.Context(), &workspaces.KubernetesConnection{Context: contextName}, resourcegroup)
	if err != nil {
		return err
	}

	output.LogInfo("resource group %q created", id)
	return nil
}
