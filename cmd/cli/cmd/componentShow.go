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
	env, err := rad.RequireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := rad.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	componentName, err := rad.RequireComponent(cmd, args)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(env)
	if err != nil {
		return err
	}

	return client.ShowComponent(cmd.Context(), applicationName, componentName)
}
