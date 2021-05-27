// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/spf13/cobra"
)

// appShowCmd command to show properties of an application
var appShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show RAD application details",
	Long:  "Show RAD application details",
	RunE:  showApplication,
}

func init() {
	applicationCmd.AddCommand(appShowCmd)

	appShowCmd.Flags().StringP("name", "n", "", "The application name")
}

func showApplication(cmd *cobra.Command, args []string) error {
	applicationName, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}

	env, err := validateDefaultEnvironment()
	if err != nil {
		return err
	}

	if applicationName == "" {
		// Get the default application name if not passed in
		applicationName = env.GetDefaultApplication()
		if applicationName == "" {
			return fmt.Errorf("No application name provided and no default application set. " +
				"Either pass in an application name or set a default application by calling `rad appplication switch`.")
		}
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)
	ac := radclient.NewApplicationClient(con, env.SubscriptionID)
	response, err := ac.Get(cmd.Context(), env.ResourceGroup, applicationName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	applicationResource := *response.ApplicationResource
	applicationDetails, err := json.MarshalIndent(applicationResource, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal application response as JSON %w", err)
	}
	fmt.Println(string(applicationDetails))

	return err
}
