// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/spf13/cobra"
)

var applicationCmd = &cobra.Command{
	Use:   "application",
	Short: "Manage applications",
	Long:  `Manage applications`,
}

func init() {
	RootCmd.AddCommand(applicationCmd)
	applicationCmd.PersistentFlags().StringP("application", "a", "", "The application name")
	applicationCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
}

func requireApplicationArgs(cmd *cobra.Command, args []string, env *environments.AzureCloudEnvironment) (string, error) {
	applicationName, err := cmd.Flags().GetString("application")
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if args[0] != "" {
			if applicationName != "" {
				return "", fmt.Errorf("cannot specify application name via both arguments and `-a`")
			}
			applicationName = args[0]
		}
	}

	if applicationName == "" {
		applicationName = env.GetDefaultApplication()
		if applicationName == "" {
			return "", fmt.Errorf("no application name provided and no default application set, " +
				"either pass in an application name or set a default application by using `rad appplication switch`")
		}
	}

	return applicationName, nil
}

func requireApplication(cmd *cobra.Command, env *environments.AzureCloudEnvironment) (string, error) {
	return requireApplicationArgs(cmd, []string{}, env)
}
