// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
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

	deploymentDeleteCmd.Flags().StringP("name", "n", "", "Deployment name")
	if err := deploymentDeleteCmd.MarkFlagRequired("name"); err != nil {
		fmt.Printf("Failed to mark the name flag required: %v", err)
	}

	deploymentDeleteCmd.Flags().StringP("application-name", "a", "", "Application name for the deployment")
	if err := deploymentDeleteCmd.MarkFlagRequired("application-name"); err != nil {
		fmt.Printf("Failed to mark the application-name flag required: %v", err)
	}
}

func deleteDeployment(cmd *cobra.Command, args []string) error {
	applicationName, err := cmd.Flags().GetString("application-name")
	if err != nil {
		return err
	}

	depName, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}

	env, err := validateDefaultEnvironment()
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain Azure credential: %w", err)
	}

	con := armcore.NewDefaultConnection(azcred, nil)

	dc := radclient.NewDeploymentClient(con, env.SubscriptionID)
	_, err = dc.Delete(cmd.Context(), env.ResourceGroup, applicationName, depName, nil)
	if err != nil {
		return fmt.Errorf("Failed to delete the deployment %s, %w", depName, err)
	}
	fmt.Printf("Deployment '%s' deleted.\n", depName)

	return err
}
