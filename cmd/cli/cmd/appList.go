// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/spf13/cobra"
)

// listCmd command to list applications deployed in the resource group
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists RAD applications",
	Long:  "Lists RAD applications deployed in all the environments in the resource group",
	Args:  cobra.ExactArgs(0),
	RunE:  listApplications,
}

const resourceType = "Applications"

func init() {
	applicationCmd.AddCommand(listCmd)
}

func listApplications(cmd *cobra.Command, args []string) error {
	env, err := validateEnvironment()
	if err != nil {
		return err
	}

	authorizer, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return err
	}

	radc := radclient.NewClient(env.SubscriptionID)
	radc.Authorizer = authorizer

	var applications []radclient.Application
	applications, err = radc.ListRadiusResources(cmd.Context(), env.ResourceGroup, resourceType)
	if err != nil {
		return fmt.Errorf("Error listing the applications: '%w'", err)
	}
	if applications == nil {
		fmt.Println("No applications found")
		return nil
	}
	for _, app := range applications {
		var applicationDetails []byte
		applicationDetails, err = json.MarshalIndent(app, "", "\t")
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", applicationDetails)
	}

	return err
}
