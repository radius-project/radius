// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environments",
	Long:  `Manage environments`,
}

func init() {
	RootCmd.AddCommand(envCmd)
	envCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
}

// Used by commands that require a named environment to be an azure cloud environment.
func validateNamedEnvironment(name string) (*environments.AzureCloudEnvironment, error) {
	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return nil, err
	}

	e, err := env.GetEnvironment(name)
	if err != nil {
		return nil, err
	}

	return environments.RequireAzureCloud(e)
}

func requireEnvironment(cmd *cobra.Command) (*environments.AzureCloudEnvironment, error) {
	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return nil, err
	}

	env, err := validateNamedEnvironment(environmentName)
	return env, err
}

func requireEnvironmentArgs(cmd *cobra.Command, args []string) (*environments.AzureCloudEnvironment, error) {
	environmentName, err := requireEnvironmentNameArgs(cmd, args)

	env, err := validateNamedEnvironment(environmentName)
	return env, err
}

func requireEnvironmentNameArgs(cmd *cobra.Command, args []string) (string, error) {
	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if environmentName != "" {
			return "", fmt.Errorf("cannot specify environment name via both arguments and `-e`")
		}
		environmentName = args[0]
	}

	return environmentName, err
}
