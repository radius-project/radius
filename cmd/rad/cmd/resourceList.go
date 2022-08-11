// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// resourceListCmd command to list resources in an application
var resourceListCmd = &cobra.Command{
	Use:     "list [resourceType]",
	Short:   "Lists resources",
	Long:    "List all resources of specified type",
	Example: `
	sample list of resourceType: containers, gateways, httpRoutes, daprPubSubBrokers, daprInvokeHttpRoutes, extenders, mongoDatabases, rabbitMQMessageQueues, redisCaches, sqlDatabases, daprStateStores, daprSecretStores

	# list all resources of a specified type in the default environment

	rad resource list containers
	rad resource list gateways
	rad resource list httpRoutes

	# list all resources of a specified type in an application
	rad resource list containers --application icecream-store
	
	# list all resources of a specified type in an application (shorthand flag)
	rad resource list containers -a icecream-store
	`,
	RunE:    listResources,
}

func init() {
	resourceCmd.AddCommand(resourceListCmd)
}

func listResources(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cmd.Flags().GetString("application")
	if err != nil {
		return &cli.FriendlyError{Message: "Unable to parse command"}
	}

	resourceType, err := cli.RequireResourceType(args)
	if err != nil {
		return &cli.FriendlyError{Message: "Unable to determine resource type"}
	}

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}

	var resourceList []generated.GenericResource
	if applicationName != "" {
		_, err := client.ShowApplication(cmd.Context(), applicationName)
		if err != nil {
			return &cli.FriendlyError{Message: fmt.Sprintf("Failed to find application %s", applicationName)}
		}
		resourceList, err = client.ListAllResourceOfTypeInApplication(cmd.Context(), applicationName, resourceType)
		if err != nil {
			return err
		}
	} else {
		resourceList, err = client.ListAllResourcesByType(cmd.Context(), resourceType)
		if err != nil {
			return err
		}
	}

	return printOutput(cmd, resourceList, false)
}

func printOutput(cmd *cobra.Command, obj interface{}, isLegacy bool) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, obj, cmd.OutOrStdout(), objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}
	return nil
}
