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

// appListCmd command to list applications deployed in the resource group
var appListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists RAD applications",
	Long:  "Lists RAD applications deployed in the resource group associated with the default environment",
	Args:  cobra.ExactArgs(0),
	RunE:  listApplications,
}

func init() {
	applicationCmd.AddCommand(appListCmd)
}

func listApplications(cmd *cobra.Command, args []string) error {
	env, err := requireEnvironment(cmd)
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)
	ac := radclient.NewApplicationClient(con, env.SubscriptionID)
	response, err := ac.ListByResourceGroup(cmd.Context(), env.ResourceGroup, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	applicationsList := *response.ApplicationList
	applications, err := json.MarshalIndent(applicationsList, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal application response as JSON %w", err)
	}
	fmt.Println(string(applications))

	return err
}
