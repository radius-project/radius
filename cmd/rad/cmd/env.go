// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environments",
	Long:  `Manage environments`,
}

func init() {
	RootCmd.AddCommand(envCmd)
	envCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
}
