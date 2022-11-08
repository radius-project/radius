// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// appShowCmd command to show properties of a  application
var appShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show RAD application details",
	Long:  "Show RAD application details",
	RunE:  showApplication,
}

func init() {
	applicationCmd.AddCommand(appShowCmd)
}

func showApplication(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	// TODO: support fallback workspace
	if !workspace.IsNamedWorkspace() {
		return workspaces.ErrNamedWorkspaceRequired
	}

	applicationName, err := cli.RequireApplicationArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	err = ShowApplication(cmd, args, *workspace, applicationName, config)
	if err != nil {
		return err
	}

	return nil
}

func ShowApplication(cmd *cobra.Command, args []string, workspace workspaces.Workspace, applicationName string, config *viper.Viper) error {
	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), workspace)
	if err != nil {
		return err
	}

	applicationResource, err := client.ShowApplication(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	return printOutput(cmd, applicationResource, false)

}
