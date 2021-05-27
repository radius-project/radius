// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
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
	Long:  `Delete the specified Radius environment`,
	RunE:  deleteEnv,
}

func init() {
	envCmd.AddCommand(envDeleteCmd)

	envDeleteCmd.Flags().StringP("name", "n", "", "The environment name")
	if err := envDeleteCmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}

	envDeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteEnv(cmd *cobra.Command, args []string) error {
	envName, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}

	noPrompt, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	// Validate environment exists, retrieve associated resource group and subscription id
	az, err := validateNamedEnvironment(envName)
	if err != nil {
		return err
	}

	if !noPrompt {
		confirmed, err := prompt.Confirm(fmt.Sprintf("Resource group %s with all its resources will be deleted. Continue deleting? [y/n]?", az.ResourceGroup))
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

	if err = deleteResourceGroup(cmd.Context(), authorizer, az.ResourceGroup, az.SubscriptionID); err != nil {
		return err
	}

	// Delete env from the config, update default env if needed
	if err = deleteEnvFromConfig(envName); err != nil {
		return err
	}

	return nil
}

// Deletes resource group and all its resources
func deleteResourceGroup(ctx context.Context, authorizer autorest.Authorizer, resourceGroup string, subscriptionID string) error {
	rgc := resources.NewGroupsClient(subscriptionID)
	rgc.Authorizer = authorizer

	// Don't timeout, let the user cancel
	rgc.PollingDuration = 0

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
