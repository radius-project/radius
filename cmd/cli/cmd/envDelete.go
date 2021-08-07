// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/prompt"
	"github.com/Azure/radius/pkg/cli/util"
	"github.com/Azure/radius/pkg/keys"
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

	config := ConfigFromContext(cmd.Context())

	// Validate environment exists, retrieve associated resource group and subscription id
	env, err := cli.RequireEnvironmentArgs(cmd, config, args)
	if err != nil {
		return err
	}

	az, ok := env.(*environments.AzureCloudEnvironment)
	if ok {

		if !yes {
			confirmed, err := prompt.Confirm(fmt.Sprintf("Resource groups %s and %s with all their resources will be deleted. Continue deleting? [y/n]?", az.ResourceGroup, az.ControlPlaneResourceGroup))
			if err != nil {
				return err
			}

			if !confirmed {
				output.LogInfo("Delete cancelled.")
				return nil
			}
		}

		authorizer, err := auth.NewAuthorizerFromCLI()
		if err != nil {
			return err
		}

		if err = deleteAllApplications(cmd.Context(), authorizer, az.ResourceGroup, az.SubscriptionID, az); err != nil {
			return err
		}

		// Delete environment, this will delete the resource group and all the resources in it
		if err = deleteRadiusResourcesInResourceGroup(cmd.Context(), authorizer, az.ResourceGroup, az.SubscriptionID); err != nil {
			return err
		}

		if err = deleteResourceGroup(cmd.Context(), authorizer, az.ControlPlaneResourceGroup, az.SubscriptionID); err != nil {
			return err
		}
	}

	output.LogInfo("Environment deleted")

	// Delete env from the config, update default env if needed
	if err = deleteEnvFromConfig(config, env.GetName()); err != nil {
		return err
	}

	return nil
}

func deleteAllApplications(ctx context.Context, authorizer autorest.Authorizer, resourceGroup string, subscriptionID string, env *environments.AzureCloudEnvironment) error {
	client, err := environments.CreateManagementClient(ctx, env)
	if err != nil {
		return err
	}

	applicationList, err := client.ListApplications(ctx)
	if err != nil {
		return err
	}

	for _, application := range applicationList.Value {
		err = appDeleteInner(ctx, client, *application.Name, env)
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteRadiusResourcesInResourceGroup(ctx context.Context, authorizer autorest.Authorizer, resourceGroup string, subscriptionID string) error {
	resourceClient := azclients.NewResourcesClient(subscriptionID, authorizer)

	// Filter for all resources by rad-environment=True and delete them.
	// Currently the custom resource provider is the only resource in the user's environment that has this tag.
	page, err := resourceClient.ListByResourceGroup(ctx, resourceGroup, "tagName eq '"+keys.TagRadiusEnvironment+"' and tagValue eq 'True'", "", nil)
	if err != nil {
		return err
	}

	for ; page.NotDone(); err = page.NextWithContext(ctx) {
		for _, r := range page.Values() {
			defaultApiVersion, err := azclients.GetDefaultAPIVersion(ctx, subscriptionID, authorizer, *r.Type)
			if err != nil {
				return err
			}
			future, err := resourceClient.Delete(ctx, resourceGroup, "", "", *r.Type, *r.Name, defaultApiVersion)
			if err != nil {
				return err
			}
			if err = future.WaitForCompletionRef(ctx, resourceClient.Client); err != nil {
				return err
			}
			res, err := future.Result(resourceClient)
			if err != nil {
				return err
			}
			if res.StatusCode != 204 {
				return fmt.Errorf("Failed to delete resource: %v", res.StatusCode)
			}
		}
	}
	return nil
}

// Deletes resource group and all its resources
func deleteResourceGroup(ctx context.Context, authorizer autorest.Authorizer, resourceGroup string, subscriptionID string) error {
	rgc := azclients.NewGroupsClient(subscriptionID, authorizer)

	output.LogInfo("Deleting resource group %v", resourceGroup)

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

	output.LogInfo("Waiting for delete to complete...")
	if err = future.WaitForCompletionRef(ctx, rgc.Client); err != nil {
		return fmt.Errorf("failed to delete the resource group: %w", err)
	}

	_, err = future.Result(rgc)
	if err != nil {
		return fmt.Errorf("failed to delete the resource group: %w", err)
	}

	return nil
}

func deleteEnvFromConfig(config *viper.Viper, envName string) error {
	output.LogInfo("Updating config")
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	delete(env.Items, envName)
	// Make another existing environment default if environment being deleted is current default
	if env.Default == envName && len(env.Items) > 0 {
		for key := range env.Items {
			env.Default = key
			output.LogInfo("%v is now the default environment", key)
			break
		}
	}
	cli.UpdateEnvironmentSection(config, env)
	if err = cli.SaveConfig(config); err != nil {
		return err
	}

	return nil
}
