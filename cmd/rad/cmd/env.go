// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(envCmd)
	envCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
	envCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}

func NewEnvironmentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Manage Radius environments",
		Long: `Manage Radius environments
Radius environments are prepared “landing zones” for Radius applications. Applications deployed to an environment will inherit the container runtime, configuration, and other settings from the environment.`,
	}
}
