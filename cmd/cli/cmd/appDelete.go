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
	"github.com/spf13/viper"
	"golang.org/x/text/cases"
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

	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplicationArgs(cmd, args, env)
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
	for _, deploymentResource := range deploymentList.Value {
		// This is needed until server side implementation is fixed https://github.com/Azure/radius/issues/159
		deploymentName := *deploymentResource.Name
		err = client.DeleteDeployment(cmd.Context(), applicationName, deploymentName)
		if err != nil {
			return fmt.Errorf("delete deployment error: %w", err)
		}
		fmt.Printf("Deployment '%s' deleted.\n", deploymentName)
	}

	err = client.DeleteApplication(cmd.Context(), applicationName)
	if err != nil {
		return fmt.Errorf("delete application error: %w", err)
	}

	err = updateApplicationConfig(config, env, applicationName)
	if err != nil {
		return err
	}

	fmt.Printf("Application '%s' has been deleted\n", applicationName)
	return nil
}

func updateApplicationConfig(config *viper.Viper, env environments.Environment, applicationName string) error {
	// If the application we are deleting is the default application, remove it
	if env.GetDefaultApplication() == applicationName {
		envSection, err := cli.ReadEnvironmentSection(config)
		if err != nil {
			return err
		}

		fmt.Printf("Removing default application '%v' from environment '%v'\n", applicationName, env.GetName())

		envSection.Items[cases.Fold().String(env.GetName())][environments.EnvironmentKeyDefaultApplication] = ""

		cli.UpdateEnvironmentSection(config, envSection)

		err = cli.SaveConfig(config)
		if err != nil {
			return err
		}
	}

	return nil
}
