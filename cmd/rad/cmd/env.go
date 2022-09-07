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
	envCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}

func NewEnvironmentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Manage RAD environment",
		// TODO: fix explanation
		Long: `The environment is a local configuration entry that stores the connection information for a Radius installation.
		You can use environments to store all of the Radius installations you interact with, and easily switch between them.`,
	}
}
