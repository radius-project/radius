// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(workspaceCmd)
}

func NewWorkspaceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "workspace",
		Short: "Manage RAD workspace",
		Long: `The workspace is a local configuration entry that stores the connection information for a Radius installation.
		You can use workspaces to store all of the Radius installations you interact with, and easily switch between them.`,
	}
}
