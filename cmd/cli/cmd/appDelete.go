// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/prompt"
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
		confirmed, err := prompt.Confirm(fmt.Sprintf("Are you sure you want to delete '%v' from '%v' [y/n]?", applicationName, env.GetName()))
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

	deploymentList, err := client.ListDeployments(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	// Delete the deployments
	for _, deploymentResource := range *deploymentList.Value {
		// This is needed until server side implementation is fixed https://github.com/Azure/radius/issues/159
		deploymentName := *deploymentResource.Name
		err = client.DeleteDeployment(cmd.Context(), deploymentName, applicationName)
		if err != nil {
			return err
		}
		fmt.Printf("Deployment '%s' deleted.\n", deploymentName)
	}

	err = client.DeleteApplication(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	err = updateApplicationConfig(env, applicationName)
	if err != nil {
		return err
	}

	fmt.Printf("Application '%s' has been deleted\n", applicationName)
	return nil
}

func updateApplicationConfig(env environments.Environment, applicationName string) error {
	// If the application we are deleting is the default application, remove it
	if env.GetDefaultApplication() == applicationName {
		v := viper.GetViper()
		envSection, err := rad.ReadEnvironmentSection(v)
		if err != nil {
			return err
		}

		fmt.Printf("Removing default application '%v' from environment '%v'\n", applicationName, env.GetName())

		envSection.Items[env.GetName()][environments.EnvironmentKeyDefaultApplication] = ""

		rad.UpdateEnvironmentSection(v, envSection)

		err = rad.SaveConfig()
		if err != nil {
			return err
		}
	}

	return nil
}
