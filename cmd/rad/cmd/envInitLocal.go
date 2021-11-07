// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/azure/selector"
	"github.com/Azure/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var envInitLocalCmd = &cobra.Command{
	Use:   "local",
	Short: "Initializes a local environment",
	Long:  `Initializes a local environment`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config := ConfigFromContext(cmd.Context())

		interactive, err := cmd.Flags().GetBool("interactive")
		if err != nil {
			return err
		}

		subscriptionID := ""
		resourceGroup := ""
		if interactive {
			confirm, err := prompt.Confirm("Add Azure subscription? [y/n]")
			if err != nil {
				return err
			}

			if confirm {
				sub, rg, err := selector.Select(cmd.Context())
				if err != nil {
					return err
				}

				subscriptionID = sub
				resourceGroup = rg
			}
		}

		err = createLocalEnvironment(config, subscriptionID, resourceGroup)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	envInitCmd.AddCommand(envInitLocalCmd)

	envInitLocalCmd.Flags().BoolP("interactive", "i", false, "use interactive mode")
}

func createLocalEnvironment(config *viper.Viper, subscriptionID string, resourceGroup string) error {
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	definition := map[string]interface{}{
		"kind": "local",
	}

	if subscriptionID != "" && resourceGroup != "" {
		definition["subscriptionid"] = subscriptionID
		definition["resourcegroup"] = resourceGroup
	}

	env.Items["local"] = definition

	env.Default = "local"
	cli.UpdateEnvironmentSection(config, env)

	err = cli.SaveConfig(config)
	if err != nil {
		return err
	}

	return nil
}
