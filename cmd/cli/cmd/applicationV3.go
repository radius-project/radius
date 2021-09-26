// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var applicationV3Cmd = &cobra.Command{
	Use:   "applicationV3",
	Short: "Manage RADv3 applications",
	Long:  `Manage RADv3 applications`,
}

func init() {
	RootCmd.AddCommand(applicationV3Cmd)
	applicationV3Cmd.PersistentFlags().StringP("application", "a", "", "The application name")
	applicationV3Cmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
}
