// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/objectformats"
	"github.com/Azure/radius/pkg/rad/output"
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

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	componentList, err := client.ListComponents(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	format, err := rad.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, componentList.Value, cmd.OutOrStdout(), objectformats.GetComponentTableFormat())
	if err != nil {
		return err
	}

	return nil
}
