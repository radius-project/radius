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

	env, err := validateEnvironment()
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("failed to obtain a Azure credential: %w", err)
	}

	con := armcore.NewDefaultConnection(azcred, nil)

	// Delete deployments: An application can have multiple deployments in it that should be deleted before the application can be deleted.
	dc := radclient.NewDeploymentClient(con, env.SubscriptionID)
	_, err = dc.Delete(cmd.Context(), env.ResourceGroup, applicationName, depName, nil)
	if err != nil {
		return fmt.Errorf("Failed to delete the deployment %s, %w", depName, err)
	}
	fmt.Printf("Deployment '%s' deleted.\n", depName)

	// Retrieve all the deployments in the application
	// response, err := dc.ListByApplication(cmd.Context(), env.ResourceGroup, applicationName, nil)
	// if err != nil {
	// 	return err
	// }

	// Delete the deployments
	// deploymentResources := *response.DeploymentList
	// for _, deploymentResource := range *deploymentResources.Value {
	// 	// This is needed until server side implementation is fixed https://github.com/Azure/radius/issues/159
	// 	deploymentName := utils.GetResourceNameFromFullyQualifiedPath(*deploymentResource.Name)

	// 	_, err := dc.Delete(cmd.Context(), env.ResourceGroup, applicationName, deploymentName, nil)
	// 	if err != nil {
	// 		return fmt.Errorf("Failed to delete the deployment %s, %w", deploymentName, err)
	// 	}
	// 	fmt.Printf("Deleted deployment '%s'\n", deploymentName)
	// }

	// Delete application
	// ac := radclient.NewApplicationClient(con, env.SubscriptionID)

	// _, err = ac.Delete(cmd.Context(), env.ResourceGroup, applicationName, nil)
	// if err != nil {
	// 	return fmt.Errorf("Failed to delete the application %w", err)
	// }
	// fmt.Printf("Application '%s' has been deleted\n", applicationName)

	return err
}
