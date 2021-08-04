// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
)

// appSwitchCmd command to switch applications
var appSwitchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Switch the default RAD application",
	Long:  "Switches the default RAD application",
	RunE:  switchApplications,
}

func init() {
	applicationCmd.AddCommand(appSwitchCmd)
}

func switchApplications(cmd *cobra.Command, args []string) error {
	applicationName, err := cmd.Flags().GetString("application")
	if err != nil {
		return err
	}

	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	if len(args) > 0 {
		if applicationName != "" {
			return fmt.Errorf("cannot specify application name via both arguments and `-a`")
		}
		applicationName = args[0]
	}

	if applicationName == "" {
		return fmt.Errorf("no application specified")
	}

	config := ConfigFromContext(cmd.Context())
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	if environmentName == "" {
		environmentName = env.Default
	}

	if environmentName == "" {
		return errors.New("no environment set, run 'rad env switch'")
	}

	e, err := env.GetEnvironment(environmentName)
	if err != nil {
		return err
	}

	if e.GetDefaultApplication() == applicationName {
		output.LogInfo("Default application is already set to %v", applicationName)
		return nil
	}

	if e.GetDefaultApplication() != "" {
		output.LogInfo("Switching default application from %v to %v", e.GetDefaultApplication(), applicationName)
	} else {
		output.LogInfo("Switching default application to %v", applicationName)
	}

	env.Items[cases.Fold().String(environmentName)][environments.EnvironmentKeyDefaultApplication] = applicationName

	cli.UpdateEnvironmentSection(config, env)
	err = cli.SaveConfig(config)
	return err
}
