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

var appListCmd = &cobra.Command{
	Use:   "application list",
	Short: "Lists the applications in the provided environment",
	Long:  "Lists the applications in the provided environment",
	RunE:  runAppList,
}

func init() {
	RootCmd.AddCommand(appListCmd)

	appListCmd.Flags().String("host", "localhost:5000", "specify the hostname (defaults to localhost:5000)")
	appListCmd.Flags().BoolP("verbose", "v", false, "output verbose logging output")
}

func runAppList(cmd *cobra.Command, args []string) error {
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

	options := &armcore.ConnectionOptions{Logging: azcore.LogOptions{IncludeBody: verbose}}
	connection := armcore.NewConnection(fmt.Sprintf("http://%s/", hostname), &radtest.AnonymousCredential{}, options)

	client := radclient.NewApplicationClient(connection, radtest.TestSubscriptionID)
	response, err := client.ListByResourceGroup(cmd.Context(), radtest.TestResourceGroup, nil)
	if err != nil {
		return err
	}

	applicationsList := *response.ApplicationList
	applications, err := json.MarshalIndent(applicationsList, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal application response as JSON %w", err)
	}
	fmt.Println(string(applications))

	return nil
}
