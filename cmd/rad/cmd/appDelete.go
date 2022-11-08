// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// appDeleteCmd command to delete an application
var appDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete RAD application",
	Long:  "Delete the specified RAD application deployed in the default environment",
	RunE:  deleteApplication,
}

func init() {
	applicationCmd.AddCommand(appDeleteCmd)

	appDeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteApplication(cmd *cobra.Command, args []string) error {
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

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

	// Prompt user to confirm deletion
	if !yes {
		confirmed, err := prompt.ConfirmWithDefault(fmt.Sprintf("Are you sure you want to delete '%v' from '%v' [y/N]?", applicationName, workspace.Name), prompt.No)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	err = DeleteApplication(cmd.Context(), *workspace, applicationName)
	if err != nil {
		return err
	}

	return nil
}

func DeleteApplication(ctx context.Context, workspace workspaces.Workspace, applicationName string) error {
	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(ctx, workspace)
	if err != nil {
		return err
	}

	deleted, err := client.DeleteApplication(ctx, applicationName)
	if err != nil {
		return err
	}

	if deleted {
		output.LogInfo("Application deleted")
	} else {
		output.LogInfo("Application '%s' does not exist or has already been deleted.", applicationName)
	}

	return nil
}
