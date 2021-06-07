// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
)

// componentListCmd command to list components in an application
var componentListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists application components",
	Long:  "List all the components in the specified application",
	RunE:  listComponents,
}

func init() {
	componentCmd.AddCommand(componentListCmd)
}

func listComponents(cmd *cobra.Command, args []string) error {
	env, err := rad.RequireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := rad.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(env)
	if err != nil {
		return err
	}

	return client.ListComponents(cmd.Context(), applicationName)
}
