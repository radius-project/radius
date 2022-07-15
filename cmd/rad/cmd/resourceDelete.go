// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/spf13/cobra"
)

// resourceDeleteCmd is the command to delete a resource
var resourceDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a RAD resource",
	Long:  "Deletes a RAD resource with the given name",
	RunE:  deleteResource,
}

func init() {
	resourceCmd.AddCommand(resourceDeleteCmd)
}

func deleteResource(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
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

	_, err = client.DeleteResource(cmd.Context(), resourceType, resourceName)
	if err != nil {
		return err
	}

	return nil
}
