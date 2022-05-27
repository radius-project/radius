// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// resourceListCmd command to list resources in an application
var resourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists application resources",
	Long:  "List all the resources in the specified application",
}

func init() {
	if CLIVERSION == "v20220315privatepreview" {
		resourceListCmd.RunE = listResourcesv20220315privatepreview
	} else {
		resourceListCmd.RunE = listResources
	}
	resourceCmd.AddCommand(resourceListCmd)
}

var listResources = func(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	resourceList, err := client.ListAllResourcesByApplication(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, resourceList.Value, cmd.OutOrStdout(), objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	return nil
}

var listResourcesv20220315privatepreview = func(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	_, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}
	return nil
}
