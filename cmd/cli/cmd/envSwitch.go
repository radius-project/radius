// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/logger"
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
	section, err := rad.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	if len(section.Items) == 0 {
		fmt.Println("No environments found. Use 'rad env init' to initialize.")
		return nil
	}

	envName, err := rad.RequireEnvironmentNameArgs(cmd, args)
	if err != nil {
		return err
	}

	_, ok := section.Items[envName]
	if !ok {
		fmt.Printf("Could not find environment %v\n", envName)
		return nil
	}

	// Retrieve associated resource group and subscription id
	env, err := rad.ValidateNamedEnvironment(config, envName)
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

	logger.LogInfo(text)

	section.Default = envName
	rad.UpdateEnvironmentSection(config, section)
	err = rad.SaveConfig(config)
	if err != nil {
		return err
	}

	return nil
}
