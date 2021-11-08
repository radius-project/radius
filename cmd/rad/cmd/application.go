// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var applicationCmd = &cobra.Command{
	Use:     "application",
	Short:   "Manage RAD applications",
	Long:    `Manage RAD applications`,
	Aliases: []string{"app"},
}

func init() {
	RootCmd.AddCommand(applicationCmd)
	applicationCmd.PersistentFlags().StringP("application", "a", "", "The application name")
	applicationCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
}
