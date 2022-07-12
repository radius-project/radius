// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall radius for a specific platform",
	Long:  `Uninstall radius for a specific platform`,
}

func init() {
	RootCmd.AddCommand(uninstallCmd)
}
