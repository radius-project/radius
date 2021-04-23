// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"

	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/rad"
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

	_, ok := env.Items[args[0]]
	if !ok {
		fmt.Printf("Could not find environment %v\n", args[0])
		return nil
	}

	envUrl := utils.GenerateEnvUrl(env.Items[args[0]]["kind"].(string), env.Items[args[0]]["subscriptionid"].(string), env.Items[args[0]]["resourcegroup"].(string)) 
	
	logger.LogInfo("Default environment is now: %v\n\n" +
				   "%v environment is available at:\n%v\n", args[0], args[0], envUrl)		   

	env.Default = args[0]
	rad.UpdateEnvironmentSection(v, env)
	err = saveConfig()
	if err != nil {
		return err
	}

	return nil
}
