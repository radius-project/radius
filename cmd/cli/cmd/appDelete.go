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
	"github.com/Azure/radius/pkg/rad/prompt"
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

	appDeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteApplication(cmd *cobra.Command, args []string) error {
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	env, err := rad.RequireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := rad.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	// Prompt user to confirm deletion
	if !yes {
		confirmed, err := prompt.Confirm(fmt.Sprintf("Are you sure you want to delete '%v' from '%v' [y/n]?", applicationName, env.Name))
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("failed to obtain Azure credential: %w", err)
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

	err = updateApplicationConfig(cmd, env, applicationName, ac)
	return err
}

func updateApplicationConfig(cmd *cobra.Command,
	azureEnv *environments.AzureCloudEnvironment,
	applicationName string,
	ac *radclient.ApplicationClient) error {

	// If the application we are deleting is the default application, remove it
	if azureEnv.DefaultApplication == applicationName {
		v := viper.GetViper()
		env, err := rad.ReadEnvironmentSection(v)
		if err != nil {
			return err
		}

		fmt.Printf("Removing default application '%v' from environment '%v'\n", applicationName, azureEnv.Name)

		env.Items[azureEnv.Name][environments.DefaultApplication] = ""

		rad.UpdateEnvironmentSection(v, env)

		err = saveConfig()
		if err != nil {
			return err
		}
	}

	return nil
}
