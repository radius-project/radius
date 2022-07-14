// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var workspaceInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize local workspace",
	Long:  `Initialize local workspace`,
}

func init() {
	workspaceCmd.AddCommand(workspaceInitCmd)
}
