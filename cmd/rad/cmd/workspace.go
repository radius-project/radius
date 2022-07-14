// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import "github.com/spf13/cobra"

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage local workspaces",
	Long: `The workspace is a local configuration entry that stores the connection information for a Radius installation.
You can use workspaces to store all of the Radius installations you interact with, and easily switch between them.`,
}

func init() {
	RootCmd.AddCommand(workspaceCmd)
	workspaceCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}
