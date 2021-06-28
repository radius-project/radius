// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad"
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
	envName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	config := ConfigFromContext(cmd.Context())
	env, err := rad.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	e, err := env.GetEnvironment(envName)
	if err != nil {
		return err
	}

	b, err := yaml.Marshal(&e)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(string(b))
	fmt.Println()
	return nil

}
