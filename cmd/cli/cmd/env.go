// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"

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
}

// Used by commands that require the current environment to be an azure cloud environment.
func validateDefaultEnvironment() (*environments.AzureCloudEnvironment, error) {
	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return nil, err
	}

	if env.Default == "" {
		return nil, errors.New("no environment set, run 'rad env switch'")
	}

	e, err := env.GetEnvironment("") // default environment
	if err != nil {
		return nil, err
	}

	return environments.RequireAzureCloud(e)
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
