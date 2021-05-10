// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
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
	if len(args) < 1 {
		return errors.New("environment name is required")
	}

	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return err
	}

	if len(env.Items) == 0 {
		fmt.Println("No environments found. Use 'rad env init' to initialize.")
		return nil
	}

	name := args[0]
	_, ok := env.Items[name]
	if !ok {
		fmt.Printf("Could not find environment %v\n", name)
		return nil
	}

	// Retrieve associated resource group and subscription id
	az, err := validateNamedEnvironment(name)
	if err != nil {
		return err
	}

	envUrl, err := azure.GenerateAzureEnvUrl(az.SubscriptionID, az.ResourceGroup) 
	if err != nil {
		return err
	}
	
	logger.LogInfo("Default environment is now: %v\n\n" +
				   "%v environment is available at:\n%v\n", name, name, envUrl)		   

	env.Default = name
	rad.UpdateEnvironmentSection(v, env)
	err = saveConfig()
	if err != nil {
		return err
	}

	return nil
}
