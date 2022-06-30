// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/environments"
)

// resourceShowCmd command to show details of a resource
var resourceShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show RAD resource details",
	Long:  "Show details of the specified Radius resource",
	RunE:  showResource,
}

func init() {
	resourceShowCmd.PersistentFlags().StringP("type", "t", "", "The resource type")
	resourceShowCmd.PersistentFlags().StringP("resource", "r", "", "The resource name")
	resourceShowCmd.Flags().StringP("resource-group", "g", "", "Resource Group of the resource. This parameter is required if the resource type is a Microsoft Azure resource.")
	resourceShowCmd.Flags().StringP("resource-subscription-id", "s", "", "Subscription id of the resource. This parameter is required if the resource type is a Microsoft Azure resource.")
	resourceCmd.AddCommand(resourceShowCmd)
}

func showResource(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	if env.GetEnableUCP() {
		err := showResourceUCP(cmd, args, env)
		if err != nil {
			return err
		}
	} else {
		err := showResourceLegacy(cmd, args, env)
		if err != nil {
			return err
		}
	}

	return nil
}

func showResourceLegacy(cmd *cobra.Command, args []string, env environments.Environment) error {
	applicationName, err := cli.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	azureResource, err := isAzureConnectionResource(cmd, args)
	if err != nil {
		return err
	}
	var resourceType, resourceName, resourceGroup, resourceSubscriptionID string
	if azureResource {
		azureResource, err := cli.RequireAzureResource(cmd, args)
		if err != nil {
			return err
		}
		resourceName = azureResource.Name
		resourceType = azureResource.ResourceType
		resourceGroup = azureResource.ResourceGroup
		resourceSubscriptionID = azureResource.SubscriptionID
	} else {
		resourceType, resourceName, err = cli.RequireResource(cmd, args)
		if err != nil {
			return err
		}
	}

	client, err := environments.CreateLegacyManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	resource, err := client.ShowResource(cmd.Context(), applicationName, resourceType, resourceName, resourceGroup, resourceSubscriptionID)
	if err != nil {
		return err
	}

	return printOutput(cmd, resource, true)
}

func showResourceUCP(cmd *cobra.Command, args []string, env environments.Environment) error {
	applicationName, err := cli.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateApplicationsManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	resourceType, err := cli.RequireResourceType(args)
	if err != nil {
		return err
	}

	resourceList, err := client.ShowResourceByApplication(cmd.Context(), applicationName, resourceType)
	if err != nil {
		return err
	}

	return printOutput(cmd, resourceList, false)
}

func isAzureConnectionResource(cmd *cobra.Command, args []string) (bool, error) {
	resourceType, err := cmd.Flags().GetString("type")
	if err != nil {
		return false, err
	}

	if resourceType == "" {
		if len(args) > 0 {
			resourceType = args[0]
		} else {
			return false, fmt.Errorf("resource type is required")
		}
	}

	if azure.KnownAzureResourceType(resourceType) {
		return true, nil
	}

	return false, nil
}
