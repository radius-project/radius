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

// deploymentShowCmd command to show details of a deployment
var deploymentShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show Radius deployment details",
	Long:  "Show details of the specified Radius deployment deployed in the default environment",
	RunE:  showDeployment,
}

func init() {
	deploymentCmd.AddCommand(deploymentShowCmd)
}

func showDeployment(cmd *cobra.Command, args []string) error {
	env, err := rad.RequireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := rad.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	deploymentName, err := rad.RequireDeployment(cmd, args)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(env)
	if err != nil {
		return err
	}

	deploymentResource, err := client.ShowDeployment(cmd.Context(), deploymentName, applicationName)
	if err != nil {
		return err
	}

	deploymentDetails, err := json.MarshalIndent(deploymentResource, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal deployment response as JSON %w", err)
	}

	fmt.Println(string(deploymentDetails))
	return nil
}
