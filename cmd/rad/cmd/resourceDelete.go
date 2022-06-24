// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/spf13/cobra"
)

// resourceDeleteCmd command to show details of a resource
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
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	isUCPEnabled := false
	if env.GetKind() == environments.KindKubernetes {
		isUCPEnabled = env.(*environments.KubernetesEnvironment).GetEnableUCP()
	}

	if !isUCPEnabled {
		return errors.New("Delete is not enabled")
	}

	client, err := environments.CreateApplicationsManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	resourceType, resourceName, err := cli.RequireResourceTypeAndName(args)
	if err != nil {
		return err
	}

	deleteResponse, err := client.DeleteResource(cmd.Context(), resourceType, resourceName)
	if err != nil {
		return err
	}

	return printOutput(cmd, deleteResponse, false)
}
