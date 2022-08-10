// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
)

// resourceShowCmd command to show details of a resource
var resourceShowCmd = &cobra.Command{
	Use:     "show [resourceType] [resourceName]",
	Short:   "Show RAD resource details",
	Long:    "Show details of the specified Radius resource",
	Example: `
	sample list of resourceType: containers, gateways, httpRoutes, daprPubSubBrokers, daprInvokeHttpRoutes, extenders, mongoDatabases, rabbitMQMessageQueues, redisCaches, sqlDatabases, daprStateStores, daprSecretStores,

	# show details of a particular resource (basic)

	rad resource show containers orders
	rad resource show gateways orders_gateways
	rad resource show httpRoutes orders_routes

	# show details of a particular resource in an application
	rad resource show containers orders --application icecream-store
	
	# show details of a particular resource in an application (shorthand flag)
	rad resource show containers orders -a icecream-store 
	`,
	RunE:    showResource,
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
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplication(cmd, *workspace)
	if err != nil {
		return err
	}

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}

	resourceType, resourceName, err := cli.RequireResourceTypeAndName(args)
	if err != nil {
		return err
	}

	resourceDetails, err := client.ShowResourceByApplication(cmd.Context(), applicationName, resourceType, resourceName)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, resourceDetails, cmd.OutOrStdout(), objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
