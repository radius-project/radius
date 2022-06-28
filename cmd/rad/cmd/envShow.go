// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
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

	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	isUCPEnabled := false
	if env.GetKind() == environments.KindKubernetes {
		isUCPEnabled = env.(*environments.KubernetesEnvironment).GetEnableUCP()
	}

	envName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	envconfig, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	if isUCPEnabled {
		client, err := environments.CreateApplicationsManagementClient(cmd.Context(), env)
		if err != nil {
			return err
		}

		if envName == "" && envconfig.Default == "" {
			return errors.New("the default environment is not configured. use `rad env switch` to change the selected environment.")
		}

		if envName == "" {
			envName = envconfig.Default
		}

		envResource, err := client.GetEnvDetails(cmd.Context(), envName)
		if err != nil {
			return err
		}

		b, err := yaml.Marshal(envResource)
		if err != nil {
			return err
		}
		fmt.Println(string(b))

	} else {

		e, err := envconfig.GetEnvironment(envName)
		if err != nil {
			return err
		}

		b, err := yaml.Marshal(&e)
		if err != nil {
			return err
		}

		fmt.Println(string(b))

	}
	return nil

}
