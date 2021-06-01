// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var deploymentCmd = &cobra.Command{
	Use:   "deployment",
	Short: "Manage deployments",
	Long:  `Manage deployments`,
}

func init() {
	RootCmd.AddCommand(deploymentCmd)
	deploymentCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
	deploymentCmd.PersistentFlags().StringP("application", "a", "", "The application name")
	deploymentCmd.PersistentFlags().StringP("deployment", "d", "", "The deployment name")

}
