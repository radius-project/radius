// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/output"
)

var envDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete environment",
	Long:  `Delete the specified Radius environment`,
	RunE:  deleteEnvResource,
}

func init() {
	envCmd.AddCommand(envDeleteCmd)

	envDeleteCmd.Flags().BoolP("yes", "y", false, "Use this flag to prevent prompt for confirmation")
}

func deleteEnvResource(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	envName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	envconfig, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	if envName == "" && envconfig.Default == "" {
		return errors.New("the default environment is not configured. use `rad env switch` to change the selected environment.")
	}

	if envName == "" {
		envName = envconfig.Default
	}
	client, err := environments.CreateApplicationsManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}
	err = client.DeleteEnv(cmd.Context(), envName)
	if err == nil {
		output.LogInfo("Environment deleted")
	}
	return err

}
