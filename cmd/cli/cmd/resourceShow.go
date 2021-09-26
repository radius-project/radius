// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/objectformats"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/spf13/cobra"
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
	resourceCmd.AddCommand(resourceShowCmd)
}

func showResource(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	resourceType, resourceName, err := cli.RequireResource(cmd, args)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	resource, err := client.ShowResource(cmd.Context(), applicationName, resourceType, resourceName)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, resource, cmd.OutOrStdout(), objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	return nil
}
