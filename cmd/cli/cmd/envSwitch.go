// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return err
	}

	if len(env.Items) == 0 {
		fmt.Println("No environments found. Use 'rad env init' to initialize.")
		return nil
	}

	envName, err := requireEnvironmentNameArgs(cmd, args)
	if err != nil {
		return err
	}

	_, ok := env.Items[envName]
	if !ok {
		fmt.Printf("Could not find environment %v\n", envName)
		return nil
	}

	// Retrieve associated resource group and subscription id
	az, err := validateNamedEnvironment(envName)
	if err != nil {
		return err
	}

	envUrl, err := azure.GenerateAzureEnvUrl(az.SubscriptionID, az.ResourceGroup)
	if err != nil {
		return err
	}

	logger.LogInfo("Default environment is now: %v\n\n"+
		"%v environment is available at:\n%v\n", envName, envName, envUrl)

	env.Default = envName
	rad.UpdateEnvironmentSection(v, env)
	err = saveConfig()
	if err != nil {
		return err
	}

	return nil
}
