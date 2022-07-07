// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/k3d"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/keys"
	"github.com/project-radius/radius/pkg/kubernetes/kubectl"
)

var envDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete environment",
	Long:  `Delete the specified Radius environment`,
	RunE:  deleteEnvResource,
}

func init() {
	envCmd.AddCommand(envDeleteCmd)

	envDeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteEnvResource(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	if env.GetEnableUCP() {
		envName, err := cmd.Flags().GetString("environment")
		if err != nil {
			return err
		}

		envconfig, err := cli.ReadEnvironmentSection(config)
		if err != nil {
			return err
		}

		if envName == "" && envconfig.Default == "" {
			return errors.New("the default environment is not configured. use `rad env switch` to change the selected environment.")
		}

		if envName == "" {
			envName = envconfig.Default
		}
		client, err := environments.CreateApplicationsManagementClient(cmd.Context(), env)
		if err != nil {
			return err
		}
		err = client.DeleteEnv(cmd.Context(), envName)
		if err == nil {
			output.LogInfo("Environment deleted")
		} else {
			return err
		}

		// Delete env from the config, update default env if needed
		err = deleteEnvFromConfig(cmd.Context(), config, env.GetName())
		if err == nil {
			output.LogInfo("Environment removed from config.yaml")

		}
		return err
	} else {
		return deleteEnvLegacy(cmd, args)
	}
}

func deleteEnvLegacy(cmd *cobra.Command, args []string) error {
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
			confirmed, err := prompt.ConfirmWithDefault(fmt.Sprintf("All radius-created resources in resource-group %s will be deleted. Continue deleting? [y/N]?", az.ResourceGroup), prompt.No)
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
		if err = deleteAllApplications(cmd.Context(), az); err != nil {
			if err, ok := err.(*radclient.RadiusError); ok && err.Code == "EnvironmentNotFound" {
				output.LogInfo("Environment '%s' not found", az.Name)

				errDelConfig := deleteEnvFromConfig(cmd.Context(), config, env.GetName())
				if errDelConfig != nil {
					return errDelConfig
				}
				return nil
			}
			return err
		}

		if err = deleteRadiusResourcesInResourceGroup(cmd.Context(), authorizer, az.ResourceGroup, az.SubscriptionID); err != nil {
			return err
		}

		output.LogInfo("Environment deleted")
		// Deletes environment entries from rad config and context from kube config
		if err = deleteFromConfig(cmd, config, az.ClusterName, az.ResourceGroup); err != nil {
			return err
		}

		return nil
	}

	dev, ok := env.(*environments.LocalEnvironment)
	if ok {

		if !yes {
			confirmed, err := prompt.ConfirmWithDefault(fmt.Sprintf("Local K3d cluster %s will be deleted. Continue deleting? [y/N]?", dev.ClusterName), prompt.No)
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

	kub, ok := env.(*environments.KubernetesEnvironment)
	if ok {
		if !yes {
			confirmed, err := prompt.Confirm(fmt.Sprintf("Environment %s and all applications will be deleted. Continue deleting? [y/n]?", kub.Name))
			if err != nil {
				return err
			}

			if !confirmed {
				output.LogInfo("Delete cancelled.")
				return nil
			}
		}

		if err = helm.UninstallOnCluster(kub.Context); err != nil {
			return err
		}
	}

	output.LogInfo("Environment deleted")

	// Delete env from the config, update default env if needed
	if err = deleteEnvFromConfig(cmd.Context(), config, env.GetName()); err != nil {
		return err
	}

	return nil
}

// deleteAllApplications deletes all applications from a resource group.
func deleteAllApplications(ctx context.Context, env environments.Environment) error {
	client, err := environments.CreateLegacyManagementClient(ctx, env)
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

func deleteEnvFromConfig(ctx context.Context, config *viper.Viper, envName string) error {
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

	if err = cli.SaveConfigOnLock(ctx, config, cli.UpdateEnvironmentWithLatestConfig(env, cli.MergeDeleteEnvConfig(envName))); err != nil {
		return err
	}

	return nil
}

func deleteFromConfig(cmd *cobra.Command, config *viper.Viper, clusterName string, resourceGroup string) error {
	var envNames []string
	var params []string

	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	for envName, envs := range env.Items {
		if fmt.Sprint(envs["context"]) == clusterName {
			envNames = append(envNames, envName)
		}
	}
	for _, envName := range envNames {
		// Delete env from the rad config, update default env if needed
		if err = deleteEnvFromConfig(cmd.Context(), config, envName); err != nil {
			return err
		}
	}

	output.LogInfo("Updating kube config")
	params = append(params, strings.Join([]string{"users.clusterUser_", resourceGroup, "_", clusterName}, ""))
	params = append(params, strings.Join([]string{"contexts.", clusterName}, ""))
	params = append(params, strings.Join([]string{"clusters.", clusterName}, ""))

	for _, param := range params {
		if err := kubectl.RunCLICommandSilent("config", "unset", param); err != nil {
			return err
		}
	}

	return nil
}
