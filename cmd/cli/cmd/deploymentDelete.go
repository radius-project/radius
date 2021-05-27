// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/Azure/radius/pkg/radclient"
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
	envDeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteDeployment(cmd *cobra.Command, args []string) error {
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	env, err := requireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := requireApplication(cmd, env)
	if err != nil {
		return err
	}

	depName, err := requireDeployment(cmd, args)
	if err != nil {
		return err
	}

	// Prompt user to confirm deletion
	if !yes {
		confirmed, err := prompt.Confirm(fmt.Sprintf("Are you sure you want to delete '%v' from '%v' [y/n]?", depName, env.Name))
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain Azure credential: %w", err)
	}

	con := armcore.NewDefaultConnection(azcred, nil)

	dc := radclient.NewDeploymentClient(con, env.SubscriptionID)
	poller, err := dc.BeginDelete(cmd.Context(), env.ResourceGroup, applicationName, depName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	_, err = poller.PollUntilDone(cmd.Context(), radclient.PollInterval)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	fmt.Printf("Deployment '%s' deleted.\n", depName)

	return err
}
