// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var resourceCmd = &cobra.Command{
	Use:   "resource",
	Short: "Manage resources",
	Long:  `Manage resources`,
}

func init() {
	RootCmd.AddCommand(resourceCmd)
	resourceCmd.PersistentFlags().StringP("application", "a", "", "The application name")
	resourceCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
}
