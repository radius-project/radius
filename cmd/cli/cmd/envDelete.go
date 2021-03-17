// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var envDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete environment",
	Long:  `Delete environment`,
	Args:  cobra.ExactArgs(1),
	RunE:  deleteEnv,
}

func init() {
	envCmd.AddCommand(envDeleteCmd)
	envDeleteCmd.Flags().BoolP("yes", "y", false, "Do not prompt for confirmation")
}

func deleteEnv(cmd *cobra.Command, args []string) error {
	noPrompt, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	// Validate environment exists, retrieve associated resource group and subscription id
	rg, subid, err := validateEnvironmentExists(args[0])
	if err != nil {
		return err
	}

	if !noPrompt {
		confirmed, err := prompt.Confirm(fmt.Sprintf("%s %s %s", "Resource group", rg, "with all its resources will be deleted. Continue deleting? [y/n]?"))
		if err != nil {
			return err
		}

		if !confirmed {
			logger.LogInfo("Delete cancelled.")
			return nil
		}
	}

	// Delete environment, this will delete the resource group and all the resources in it
	authorizer, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return err
	}

	if err = deleteResourceGroup(cmd.Context(), authorizer, rg, subid); err != nil {
		return err
	}

	// Delete env from the config, update default env if needed
	if err = deleteEnvFromConfig(args[0]); err != nil {
		return err
	}

	return nil
}

// Validates environment name exists in the config.
// Returns resource group name and subscription id associated with the environment.
func validateEnvironmentExists(envName string) (string, string, error) {
	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return "", "", err
	}

	if len(env.Items) == 0 {
		return "", "", errors.New("No environments found.")
	}

	envConfig, exists := env.Items[envName]
	if !exists {
		return "", "", fmt.Errorf("Could not find the environment %s. Use 'rad env list' to list all environments.", envName)
	}

	return envConfig["resourcegroup"].(string), envConfig["subscriptionid"].(string), nil
}

// Deletes resource group and all its resources
func deleteResourceGroup(ctx context.Context, authorizer autorest.Authorizer, resourceGroup string, subscriptionID string) error {
	rgc := resources.NewGroupsClient(subscriptionID)
	rgc.Authorizer = authorizer

	logger.LogInfo("Deleting resource group %v", resourceGroup)
	future, err := rgc.Delete(ctx, resourceGroup)
	if err != nil {
		return fmt.Errorf("Failed to delete the resource group: %w", err)
	}

	logger.LogInfo("Waiting for delete to complete...")
	if err = future.WaitForCompletionRef(ctx, rgc.Client); err != nil {
		return fmt.Errorf("Failed to delete the resource group: %w", err)
	}

	_, err = future.Result(rgc)
	if err != nil {
		return fmt.Errorf("Failed to delete the resource group: %w", err)
	}

	logger.LogInfo("Environment deleted")

	return nil
}

func deleteEnvFromConfig(envName string) error {
	logger.LogInfo("Updating config")
	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return err
	}

	delete(env.Items, envName)
	// Make another existing environment default if environment being deleted is current default
	if env.Default == envName && len(env.Items) > 0 {
		for key := range env.Items {
			env.Default = key
			logger.LogInfo("%v is now the default environment", key)
			break
		}
	}
	rad.UpdateEnvironmentSection(v, env)
	if err = saveConfig(); err != nil {
		return err
	}

	return nil
}
