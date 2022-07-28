// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// resourceListCmd command to list resources in an application
var resourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists application resources",
	Long:  "List all the resources in the specified application",
	RunE:  listResources,
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

	applicationName, err := cli.RequireApplicationArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}
	resourceList, err := client.ListAllResourcesByApplication(cmd.Context(), applicationName)
	if err != nil {
		return err
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
