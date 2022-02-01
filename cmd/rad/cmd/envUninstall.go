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
	Short: "Uninstall RAD",
	Long:  `Uninstall RAD`,
}

func init() {
	envCmd.AddCommand(envUninstallCmd)
}
