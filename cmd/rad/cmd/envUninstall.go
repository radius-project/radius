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
	Short: "Uninstall a RAD Environment",
	Long:  `Uninstall a RAD Environment`,
	RunE:  envStop,
}

func init() {
	envCmd.AddCommand(envUninstallCmd)
}

func envUninstall(cmd *cobra.Command, args []string) error {
	return nil
	// uninstallClient := helm.NewUninstall(helmConf)

}
