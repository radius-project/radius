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

// deploymentGetCmd command to get details of a deployment
var deploymentGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get RAD deployment details",
	Long:  "Get details of the specified Radius deployment deployed in the default environment",
	RunE:  getDeployment,
}

func init() {
	deploymentCmd.AddCommand(deploymentGetCmd)

	deploymentGetCmd.Flags().StringP("name", "n", "", "Deployment name")
	if err := deploymentGetCmd.MarkFlagRequired("name"); err != nil {
		fmt.Printf("Failed to mark the name flag required: %v", err)
	}

	deploymentGetCmd.Flags().StringP("application-name", "a", "", "Application name for the deployment")
	if err := deploymentGetCmd.MarkFlagRequired("application-name"); err != nil {
		fmt.Printf("Failed to mark the application-name flag required: %v", err)
	}
}

func getDeployment(cmd *cobra.Command, args []string) error {
	applicationName, err := cmd.Flags().GetString("application-name")
	if err != nil {
		return err
	}

	depName, err := cmd.Flags().GetString("name")
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

	response, err := dc.Get(cmd.Context(), env.ResourceGroup, applicationName, depName, nil)
	if err != nil {
		return fmt.Errorf("Failed to get the deployment %s, %w", depName, err)
	}

	deploymentResource := *response.DeploymentResource
	deploymentDetails, err := json.MarshalIndent(deploymentResource, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal deployment response as JSON %w", err)
	}

	fmt.Println(string(deploymentDetails))

	return err
}
