// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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

func deleteEnvFromConfig(ctx context.Context, config *viper.Viper, envName string) error {
	output.LogInfo("Updating config")
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	delete(env.Items, envName)
	// Make another existing environment default if environment being deleted is current default
	if env.Default == envName && len(env.Items) > 0 {
		for key := range env.Items {
			env.Default = key
			output.LogInfo("%v is now the default environment", key)
			break
		}
	}

	if err = cli.SaveConfigOnLock(ctx, config, cli.UpdateEnvironmentWithLatestConfig(env, cli.MergeDeleteEnvConfig(envName))); err != nil {
		return err
	}

	return nil
}
