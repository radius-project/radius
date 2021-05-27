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

// componentListCmd command to list components in an application
var componentListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists application components",
	Long:  "List all the components in the specified application",
	RunE:  listComponents,
}

func init() {
	componentCmd.AddCommand(componentListCmd)
}

func listComponents(cmd *cobra.Command, args []string) error {
	env, err := requireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := requireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return fmt.Errorf("Failed to obtain a Azure credentials: %w", err)
	}
	con := armcore.NewDefaultConnection(azcred, nil)

	componentClient := radclient.NewComponentClient(con, env.SubscriptionID)

	response, err := componentClient.ListByApplication(cmd.Context(), env.ResourceGroup, applicationName, nil)
	if err != nil {
		return utils.UnwrapErrorFromRawResponse(err)
	}

	componentsList := *response.ComponentList
	components, err := json.MarshalIndent(componentsList, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to marshal component response as JSON %w", err)
	}
	fmt.Println(string(components))

	return err
}
