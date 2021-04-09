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
	"github.com/Azure/radius/pkg/radclient"
	"github.com/spf13/cobra"
)

// appGetCmd command to get properties of an application
var appGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get RAD application details",
	Long:  "Get RAD application details",
	RunE:  getApplication,
}

func init() {
	applicationCmd.AddCommand(appGetCmd)

	appGetCmd.Flags().StringP("name", "n", "", "The application name")
	if err := appGetCmd.MarkFlagRequired("name"); err != nil {
		fmt.Printf("Failed to mark the name flag required: %v", err)
	}
}

func getApplication(cmd *cobra.Command, args []string) error {
	applicationName, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}

	env, err := validateDefaultEnvironment()
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)
	ac := radclient.NewApplicationClient(con, env.SubscriptionID)
	response, err := ac.Get(cmd.Context(), env.ResourceGroup, applicationName, nil)
	if err != nil {
		return fmt.Errorf("Failed to get the application %s, %w", applicationName, err)
	}

	applicationResource := *response.ApplicationResource
	applicationDetails, err := json.MarshalIndent(applicationResource, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal application response as JSON %w", err)
	}
	fmt.Println(string(applicationDetails))

	return err
}
