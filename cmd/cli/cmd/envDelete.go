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
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/Azure/radius/pkg/rad/util"
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

	envDeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteEnv(cmd *cobra.Command, args []string) error {
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	// Validate environment exists, retrieve associated resource group and subscription id
	env, err := rad.RequireEnvironmentArgs(cmd, args)
	if err != nil {
		return err
	}

	az, err := environments.RequireAzureCloud(env)
	if err != nil {
		return err
	}

	if !yes {
		confirmed, err := prompt.Confirm(fmt.Sprintf("Resource groups %s and %s with all their resources will be deleted. Continue deleting? [y/n]?", az.ResourceGroup, az.ControlPlaneResourceGroup))
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

	if err = deleteResourceGroup(cmd.Context(), authorizer, az.ControlPlaneResourceGroup, az.SubscriptionID); err != nil {
		return err
	}

	logger.LogInfo("Environment deleted")

	// Delete env from the config, update default env if needed
	if err = deleteEnvFromConfig(az.Name); err != nil {
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

	_, err := rgc.Get(ctx, resourceGroup)
	if err != nil && util.IsAutorest404Error(err) {
		return nil
	} else if err != nil {
		return err
	}

	future, err := rgc.Delete(ctx, resourceGroup)
	if err != nil {
		return fmt.Errorf("failed to delete the resource group: %w", err)
	}

	logger.LogInfo("Waiting for delete to complete...")
	if err = future.WaitForCompletionRef(ctx, rgc.Client); err != nil {
		return fmt.Errorf("failed to delete the resource group: %w", err)
	}

	_, err = future.Result(rgc)
	if err != nil {
		return fmt.Errorf("failed to delete the resource group: %w", err)
	}

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
	if err = rad.SaveConfig(); err != nil {
		return err
	}

	return nil
}
