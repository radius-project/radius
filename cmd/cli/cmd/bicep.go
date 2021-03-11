// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var bicepCmd = &cobra.Command{
	Use:   "bicep",
	Short: "Manage bicep compiler",
	Long:  `Manage bicep compiler used by Radius`,
}

func init() {
	rootCmd.AddCommand(bicepCmd)
}
