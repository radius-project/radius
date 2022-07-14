// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

var workspaceSwitchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Switch current workspace",
	Long:  `Switch current workspace`,
	RunE:  switchWorkspace,
}

func init() {
	workspaceCmd.AddCommand(workspaceSwitchCmd)
}

func switchWorkspace(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspaceName, err := cli.ReadWorkspaceNameArgs(cmd, args)
	if err != nil {
		return err
	}

	section, err := cli.ReadWorkspaceSection(config)
	if err != nil {
		return err
	}

	if strings.EqualFold(section.Default, workspaceName) {
		output.LogInfo("Default environment is already set to %v", workspaceName)
		return nil
	}

	if section.Default == "" {
		output.LogInfo("Switching default workspace to %v", workspaceName)
	} else {
		output.LogInfo("Switching default workspace from %v to %v", section.Default, workspaceName)
	}

	err = cli.EditWorkspaces(cmd.Context(), config, func(section *cli.WorkspaceSection) error {
		section.Default = workspaceName
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
