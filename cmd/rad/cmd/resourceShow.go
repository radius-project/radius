// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
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

	resourceType, err := cli.RequireResourceType(args)
	if err != nil {
		return err
	}

	resourceDetails, err := client.ShowResourceByApplication(cmd.Context(), applicationName, resourceType)
	if err != nil {
		return err
	}

	b, err := yaml.Marshal(resourceDetails)
	if err != nil {
		return err
	}
	fmt.Println(string(b))

	return nil
}
