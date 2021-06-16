// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"encoding/json"
	"fmt"

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

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	deploymentList, err := client.ListDeployments(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	deployments, err := json.MarshalIndent(deploymentList, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal deployment response as JSON %w", err)
	}

	fmt.Println(string(deployments))

	return nil
}
