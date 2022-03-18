// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

var envSwitchCmd = &cobra.Command{
	Use:   "switch [environment]",
	Short: "Switch the current environment",
	Long:  "Switch the current environment",
	RunE:  switchEnv,
}

func init() {
	envCmd.AddCommand(envSwitchCmd)
}

func switchEnv(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	section, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	if len(section.Items) == 0 {
		fmt.Println("No environments found. Use 'rad env init' to initialize.")
		return nil
	}

	envName, err := cli.RequireEnvironmentNameArgs(cmd, args)
	if err != nil {
		return err
	}

	_, ok := section.Items[envName]
	if !ok {
		fmt.Printf("Could not find environment %v\n", envName)
		return nil
	}

	// Retrieve associated resource group and subscription id
	env, err := cli.ValidateNamedEnvironment(config, envName)
	if err != nil {
		return err
	}

	status := env.GetStatusLink()
	var text string
	if status == "" {
		text = fmt.Sprintf("Default environment is now: %v\n", envName)
	} else {
		text = fmt.Sprintf("Default environment is now: %v\n\n"+
			"%v environment is available at:\n%v\n", envName, envName, status)
	}

	output.LogInfo(text)

	section.Default = envName

	err = SaveConfig(cmd.Context(), config, UpdateEnvironmentSection(section))
	if err != nil {
		return err
	}

	return nil
}
