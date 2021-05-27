// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Manage components",
	Long:  `Manage components`,
}

func init() {
	RootCmd.AddCommand(componentCmd)
}

func requireComponent(cmd *cobra.Command, args []string) (string, error) {
	return require(cmd, args, "component")
}
