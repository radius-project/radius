// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
	Long:  `Manage workspaces`,
}

func init() {
	RootCmd.AddCommand(workspaceCmd)
	workspaceCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
}
