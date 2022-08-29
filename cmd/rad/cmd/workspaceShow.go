// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

var workspaceShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show local workspace",
	Long:  `Show local workspace`,
	RunE:  showWorkspace,
}

func init() {
	workspaceCmd.AddCommand(workspaceShowCmd)
}

func showWorkspace(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	workspace, err := cli.RequireWorkspaceArgs(cmd, config, args)
	if err != nil {
		return err
	}

	err = output.Write(format, workspace, cmd.OutOrStdout(), objectformats.GetWorkspaceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
