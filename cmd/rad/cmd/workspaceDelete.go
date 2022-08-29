// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
)

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete local workspace",
	Long:  `Delete local workspace`,
	RunE:  deleteWorkspace,
}

func init() {
	workspaceCmd.AddCommand(workspaceDeleteCmd)

	workspaceDeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteWorkspace(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())

	workspace, err := cli.RequireWorkspaceArgs(cmd, config, args)
	if err != nil {
		return err
	}

	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	// Prompt user to confirm deletion
	if !yes {
		message := fmt.Sprintf("Are you sure you want to delete workspace '%v' from local config [y/N]? This will update config but will not delete any deployed resources.", workspace.Name)
		confirmed, err := prompt.ConfirmWithDefault(message, prompt.No)
		if err != nil {
			return err
		}

		if !confirmed {
			return nil
		}
	}

	err = cli.EditWorkspaces(cmd.Context(), config, func(section *cli.WorkspaceSection) error {
		delete(section.Items, strings.ToLower(workspace.Name))
		if strings.EqualFold(section.Default, workspace.Name) {
			section.Default = ""
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
