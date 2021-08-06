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

// componentShowCmd command to show details of a component
var componentShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show RAD component details",
	Long:  "Show details of the specified Radius component",
	RunE:  showComponent,
}

func init() {
	componentCmd.AddCommand(componentShowCmd)
}

func showComponent(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	componentName, err := cli.RequireComponent(cmd, args)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	componentResource, err := client.ShowComponent(cmd.Context(), applicationName, componentName)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, componentResource, cmd.OutOrStdout(), objectformats.GetComponentTableFormat())
	if err != nil {
		return err
	}

	return nil
}
