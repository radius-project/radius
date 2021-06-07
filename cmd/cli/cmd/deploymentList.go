// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
)

// deploymentListCmd command to list deployments in an application
var deploymentListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists application deployments",
	Long:  "List all the deployments in the specified application",
	RunE:  listDeployments,
}

func init() {
	deploymentCmd.AddCommand(deploymentListCmd)
}

func listDeployments(cmd *cobra.Command, args []string) error {
	env, err := rad.RequireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := rad.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(env)
	if err != nil {
		return err
	}

	return client.ListDeployments(cmd.Context(), applicationName)
}
