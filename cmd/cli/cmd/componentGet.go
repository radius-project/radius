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

// componentGetCmd command to get details of a component
var componentGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get RAD component details",
	Long:  "Get details of the specified Radius component",
	RunE:  getComponent,
}

func init() {
	componentCmd.AddCommand(componentGetCmd)

	componentGetCmd.Flags().StringP("name", "n", "", "Component name")
	if err := componentGetCmd.MarkFlagRequired("name"); err != nil {
		fmt.Printf("Failed to mark the name flag required: %v", err)
	}

	componentGetCmd.Flags().StringP("application-name", "a", "", "Application name for the component")
	if err := componentGetCmd.MarkFlagRequired("application-name"); err != nil {
		fmt.Printf("Failed to mark the application-name flag required: %v", err)
	}
}

func getComponent(cmd *cobra.Command, args []string) error {
	applicationName, err := cmd.Flags().GetString("application-name")
	if err != nil {
		return err
	}

	componentName, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}

	env, err := validateDefaultEnvironment()
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain a Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)
	componentClient := radclient.NewComponentClient(con, env.SubscriptionID)

	response, err := componentClient.Get(cmd.Context(), env.ResourceGroup, applicationName, componentName, nil)
	if err != nil {
		return fmt.Errorf("Failed to get the component %s, %w", componentName, err)
	}

	componentResource := *response.ComponentResource
	componentDetails, err := json.MarshalIndent(componentResource, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal component response as JSON %w", err)
	}
	fmt.Println(string(componentDetails))

	return err
}
