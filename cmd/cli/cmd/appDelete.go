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
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// appDeleteCmd command to delete an application
var appDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete RAD application",
	Long:  "Delete the specified RAD application deployed in the default environment",
	RunE:  deleteApplication,
}

func init() {
	applicationCmd.AddCommand(appDeleteCmd)

	appDeleteCmd.Flags().StringP("name", "n", "", "The application name")
	if err := appDeleteCmd.MarkFlagRequired("name"); err != nil {
		fmt.Printf("Failed to mark the name flag required: %v", err)
	}
}

func deleteApplication(cmd *cobra.Command, args []string) error {
	applicationName, err := cmd.Flags().GetString("name")
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

	// Delete deployments: An application can have multiple deployments in it that should be deleted before the application can be deleted.
	dc := radclient.NewDeploymentClient(con, env.SubscriptionID)

	// Retrieve all the deployments in the application
	response, err := dc.ListByApplication(cmd.Context(), env.ResourceGroup, applicationName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	// Delete the deployments
	deploymentResources := *response.DeploymentList
	for _, deploymentResource := range *deploymentResources.Value {
		// This is needed until server side implementation is fixed https://github.com/Azure/radius/issues/159
		deploymentName := *deploymentResource.Name

		poller, err := dc.BeginDelete(cmd.Context(), env.ResourceGroup, applicationName, deploymentName, nil)
		if err != nil {
			return utils.UnwrapErrorFromRawResponse(err)
		}

		_, err = poller.PollUntilDone(cmd.Context(), radclient.PollInterval)
		if err != nil {
			return utils.UnwrapErrorFromRawResponse(err)
		}

		fmt.Printf("Deleted deployment '%s'\n", deploymentName)
	}

	// Delete application
	ac := radclient.NewApplicationClient(con, env.SubscriptionID)

	_, err = ac.Delete(cmd.Context(), env.ResourceGroup, applicationName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}
	fmt.Printf("Application '%s' has been deleted\n", applicationName)

	updateApplicationConfig(cmd, env, applicationName, ac)
	return err
}

func updateApplicationConfig(cmd *cobra.Command,
	env *environments.AzureCloudEnvironment,
	appName string,
	ac *radclient.ApplicationClient) error {
	v := viper.GetViper()
	as, err := rad.ReadApplicationSection(v)
	if err != nil {
		return err
	}

	// If the application we are deleting is the default application,
	// find another application to default to or make it empty
	if as.Default == appName {
		response, err := ac.ListByResourceGroup(cmd.Context(), env.ResourceGroup, nil)

		if err != nil {
			return err
		}

		applicationsList := *response.ApplicationList
		appList := *applicationsList.Value
		if len(appList) < 1 {
			logger.LogInfo("Removing default application")
			as.Default = ""
		} else {
			as.Default = *appList[0].Name
			logger.LogInfo("Default application is now %v.", as.Default)
		}
	}

	rad.UpdateApplicationSection(v, as)
	if err = saveConfig(); err != nil {
		return err
	}

	return nil
}
