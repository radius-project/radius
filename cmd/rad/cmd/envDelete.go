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
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/k3d"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/prompt"
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
			confirmed, err := prompt.ConfirmWithDefault(fmt.Sprintf("Resource groups %s and all radius-created resources in %s will be deleted. Continue deleting? [yN]?", az.ControlPlaneResourceGroup, az.ResourceGroup), prompt.No)
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

		// Delete the environment will consist of:
		// 1. Delete all applications
		// 2. Delete all radius resources in the customer/user resource group (ex custom resource provider)
		// 3. Delete control plane resource group
		if err = deleteAllApplications(cmd.Context(), authorizer, az.ResourceGroup, az.SubscriptionID, az); err != nil {
			return err
		}

		if err = deleteRadiusResourcesInResourceGroup(cmd.Context(), authorizer, az.ResourceGroup, az.SubscriptionID); err != nil {
			return err
		}

		if err = deleteResourceGroup(cmd.Context(), authorizer, az.ControlPlaneResourceGroup, az.SubscriptionID); err != nil {
			return err
		}
	}

	dev, ok := env.(*environments.LocalEnvironment)
	if ok {
		if !yes {
			confirmed, err := prompt.Confirm(fmt.Sprintf("Local K3d cluster %s will be deleted. Continue deleting? [y/n]?", dev.ClusterName))
			if err != nil {
				return err
			}

			if !confirmed {
				output.LogInfo("Delete cancelled.")
				return nil
			}
		}

		err := k3d.DeleteCluster(cmd.Context(), dev.ClusterName)
		if err != nil {
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

// deleteAllApplications deletes all applications from a resource group.
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

// deleteRadiusResourcesInResourceGroup deletes all radius resources from the customer/user resource group.
// Currently the custom resource provider is the only resource in the user's environment that has this tag.
func deleteRadiusResourcesInResourceGroup(ctx context.Context, authorizer autorest.Authorizer, resourceGroup string, subscriptionID string) error {
	resourceClient := clients.NewResourcesClient(subscriptionID, authorizer)

	// Filter for all resources by rad-environment=True.
	page, err := resourceClient.ListByResourceGroup(ctx, resourceGroup, "tagName eq '"+keys.TagRadiusEnvironment+"' and tagValue eq 'True'", "", nil)
	if err != nil {
		return err
	}

	for ; page.NotDone(); err = page.NextWithContext(ctx) {
		if err != nil {
			return err
		}
		for _, r := range page.Values() {
			defaultApiVersion, err := clients.GetDefaultAPIVersion(ctx, subscriptionID, authorizer, *r.Type)
			if err != nil {
				return err
			}

			output.LogInfo("Deleting radius resource %s", *r.Name)

			future, err := resourceClient.DeleteByID(ctx, *r.ID, defaultApiVersion)
			if err != nil {
				return err
			}
			if err = future.WaitForCompletionRef(ctx, resourceClient.Client); err != nil {
				return err
			}
			_, err = future.Result(resourceClient)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Deletes resource group and all its resources
func deleteResourceGroup(ctx context.Context, authorizer autorest.Authorizer, resourceGroup string, subscriptionID string) error {
	rgc := clients.NewGroupsClient(subscriptionID, authorizer)

	output.LogInfo("Deleting resource group %v", resourceGroup)
	future, err := rgc.Delete(ctx, resourceGroup)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "the resource group", err)
	}

	output.LogInfo("Waiting for delete to complete...")
	err = future.WaitForCompletionRef(ctx, rgc.Client)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "the resource group", err)
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
