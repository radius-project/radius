// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// appListCmd command to list  applications deployed in the resource group
var appListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists RAD applications",
	Long:  "Lists RAD applications deployed in the resource group associated with the default environment",
	Args:  cobra.ExactArgs(0),
	RunE:  listApplications,
}

func init() {
	applicationCmd.AddCommand(appListCmd)
}

func listApplications(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, ConfigFromContext(cmd.Context()), DirectoryConfigFromContext(cmd.Context()))
	if err != nil {
		return err
	}

	// TODO: support fallback workspace
	if !workspace.IsNamedWorkspace() {
		return workspaces.ErrNamedWorkspaceRequired
	}

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}
	applicationList, err := client.ListApplications(cmd.Context())
	if err != nil {
		return err
	}

	return printOutput(cmd, applicationList, false)
}

func printOutput(cmd *cobra.Command, obj any, isLegacy bool) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, obj, cmd.OutOrStdout(), objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}
	return nil
}
