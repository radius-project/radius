// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
)

// deploymentDeleteCmd command to delete a deployment
var deploymentDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a Radius deployment",
	Long:  "Delete the specified Radius deployment deployed in the default environment",
	RunE:  deleteDeployment,
}

func init() {
	deploymentCmd.AddCommand(deploymentDeleteCmd)
	deploymentDeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteDeployment(cmd *cobra.Command, args []string) error {
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	deploymentName, err := cli.RequireDeployment(cmd, args)
	if err != nil {
		return err
	}

	// Prompt user to confirm deletion
	if !yes {
		confirmed, err := prompt.Confirm(fmt.Sprintf("Are you sure you want to delete '%v' from '%v' [y/n]?", deploymentName, env.GetName()))
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	return client.DeleteDeployment(cmd.Context(), applicationName, deploymentName)
}
