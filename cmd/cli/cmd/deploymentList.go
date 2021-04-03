// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/spf13/cobra"
)

// deploymentListCmd command to delete a deployment
var deploymentListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists application deployments",
	Long:  "List all the deployments in the specified application",
	RunE:  listDeployments,
}

func init() {
	deploymentCmd.AddCommand(deploymentListCmd)

	deploymentListCmd.Flags().StringP("name", "n", "", "Application name")
	if err := deploymentListCmd.MarkFlagRequired("name"); err != nil {
		fmt.Printf("Failed to mark the name flag required: %v", err)
	}
}

func listDeployments(cmd *cobra.Command, args []string) error {
	applicationName, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}

	env, err := validateEnvironment()
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain a Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)
	dc := radclient.NewDeploymentClient(con, env.SubscriptionID)

	response, err := dc.ListByApplication(cmd.Context(), env.ResourceGroup, applicationName, nil)
	if err != nil {
		return fmt.Errorf("Failed to list deployments in the application %s, %w", applicationName, err)
	}

	deploymentsList := *response.DeploymentList
	deployments, err := json.MarshalIndent(deploymentsList, "", "\t")
	if err != nil {
		return fmt.Errorf("Failed to list deployments in the application %w", err)
	}

	fmt.Println(string(deployments))

	return err
}
