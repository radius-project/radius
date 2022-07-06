// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Installs radius for a given platform",
	Long:  `Installs radius for a given platform`,
}

func init() {
	RootCmd.AddCommand(installCmd)
}
