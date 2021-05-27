// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		if args[0] != "" {
			if applicationName != "" {
				return fmt.Errorf("cannot specify application name via both arguments and `-a`")
			}
			applicationName = args[0]
		}
	}

	if applicationName == "" {
		return fmt.Errorf("no application specified")
	}

	v := viper.GetViper()
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return err
	}

	if env.Default == "" {
		return errors.New("no environment set, run 'rad env switch'")
	}

	e, err := env.GetEnvironment(environmentName)
	if err != nil {
		return err
	}

	if e.GetDefaultApplication() == applicationName {
		logger.LogInfo("Default application is already set to %v", applicationName)
		return nil
	}

	azureEnv, err := environments.RequireAzureCloud(e)
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)
	ac := radclient.NewApplicationClient(con, azureEnv.SubscriptionID)

	// Need to validate that application exists prior to switching
	_, err = ac.Get(cmd.Context(), azureEnv.ResourceGroup, applicationName, nil)
	if err != nil {
		return fmt.Errorf("could not find application '%v' in environment '%v': %w", applicationName, azureEnv.Name, utils.UnwrapErrorFromRawResponse(err))
	}

	if azureEnv.DefaultApplication != "" {
		logger.LogInfo("Switching default application from %v to %v", azureEnv.DefaultApplication, applicationName)
	} else {
		logger.LogInfo("Switching default application to %v", applicationName)
	}

	env.Items[azureEnv.Name]["defaultapplication"] = applicationName

	rad.UpdateEnvironmentSection(v, env)
	err = saveConfig()
	return err
}
