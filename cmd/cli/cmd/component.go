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
	componentCmd.PersistentFlags().StringP("application", "a", "", "The application name")
	componentCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
	componentCmd.PersistentFlags().StringP("component", "c", "", "The component name")
}
