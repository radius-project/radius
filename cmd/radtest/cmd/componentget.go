// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/Azure/radius/pkg/radtest"
	"github.com/spf13/cobra"
)

var componentGetCmd = &cobra.Command{
	Use:   "component get <app name> <component name>",
	Short: "Gets the component specified from the application in the provided environment",
	Long:  "Gets the component specified from the application in the provided environment",
	RunE:  runComponentGet,
}

func init() {
	RootCmd.AddCommand(componentGetCmd)

	componentGetCmd.Flags().String("host", "localhost:5000", "specify the hostname (defaults to localhost:5000)")
	componentGetCmd.Flags().BoolP("verbose", "v", false, "output verbose logging output")
}

func runComponentGet(cmd *cobra.Command, args []string) error {
	hostname, err := cmd.Flags().GetString("host")
	if err != nil {
		return err
	}

	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return err
	}

	if verbose {
		azcore.Log().SetListener(func(lc azcore.LogClassification, s string) {
			fmt.Printf("RADClient SDK %s: %s\n", lc, s)
		})
	}

	applicationName, componentName, err := validateArgs(args)
	if err != nil {
		return err
	}

	options := &armcore.ConnectionOptions{Logging: azcore.LogOptions{IncludeBody: verbose}}
	connection := armcore.NewConnection(fmt.Sprintf("http://%s/", hostname), &radtest.AnonymousCredential{}, options)

	client := radclient.NewComponentClient(connection, radtest.TestSubscriptionID)
	response, err := client.Get(cmd.Context(), radtest.TestResourceGroup, applicationName, componentName, nil)
	if err != nil {
		return err
	}

	componentResource := *response.ComponentResource
	component, err := json.MarshalIndent(componentResource, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal application response as JSON %w", err)
	}
	fmt.Println(string(component))

	return nil
}

// Validate args and return the app and component names.
func validateArgs(args []string) (string, string, error) {
	if len(args) != 3 {
		return "", "", fmt.Errorf("Invalid arguments: %s. Specify the application and component name", args)
	}
	if args[0] != "get" {
		return "", "", fmt.Errorf("Command: %s should be of the format: 'get <app name> <component name>", args)
	}
	return args[1], args[2], nil
}
