// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// appShowCmd command to show properties of a  application
var appShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show RAD application details",
	Long:  "Show RAD application details",
	RunE:  showApplication,
}

func init() {
	applicationCmd.AddCommand(appShowCmd)
}

func showApplication(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	err = ShowApplication(cmd, args, env, applicationName, config)
	if err != nil {
		return err
	}

	return nil
}

func ShowApplication(cmd *cobra.Command, args []string, env environments.Environment, applicationName string, config *viper.Viper) error {
	client, err := environments.CreateApplicationsManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	applicationResource, err := client.ShowApplication(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	return printOutput(cmd, applicationResource, false)

}
