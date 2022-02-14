// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var envUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall radius for a specific platform",
	Long:  `Uninstall radius for a specific platform`,
}

func init() {
	envCmd.AddCommand(envUninstallCmd)
}
