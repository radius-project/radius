// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// envShowCmd command returns properties of an environment
var envShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show RAD environment details",
	Long:  "Show Radius environment details. Uses the current user's default environment by default.",
	RunE:  showEnvironment,
}

func init() {
	envCmd.AddCommand(envShowCmd)
}

func showEnvironment(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	environmentName, err := cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}

	envResource, err := client.GetEnvDetails(cmd.Context(), environmentName)
	if err != nil {
		return err
	}

	b, err := yaml.Marshal(envResource)
	if err != nil {
		return err
	}
	fmt.Println(string(b))

	return nil

}
