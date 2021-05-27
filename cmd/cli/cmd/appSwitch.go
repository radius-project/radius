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
	env, err := validateDefaultEnvironment()
	if err != nil {
		return err
	}

	if len(args) < 1 {
		return errors.New("application name is required")
	}
	applicationName := args[0]

	v := viper.GetViper()
	as, err := rad.ReadApplicationSection(v)

	if err != nil {
		return err
	}

	if as.Default == applicationName {
		logger.LogInfo("Default application is already set to %v", applicationName)
		return nil
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)
	ac := radclient.NewApplicationClient(con, env.SubscriptionID)

	// Need to validate that application exists prior to switching
	_, err = ac.Get(cmd.Context(), env.ResourceGroup, applicationName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	logger.LogInfo("Switching default application from %v to %v", as.Default, applicationName)
	as.Default = applicationName

	rad.UpdateApplicationSection(v, as)
	err = saveConfig()
	if err != nil {
		return err
	}
	return err
}
