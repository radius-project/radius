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
}

func requireDeployment(cmd *cobra.Command, args []string) (string, error) {
	return require(cmd, args, "deployment")
}
