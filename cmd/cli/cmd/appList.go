// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
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
	env, err := rad.RequireEnvironment(cmd)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(env)
	if err != nil {
		return err
	}

	applicationList, err := client.ListApplications(cmd.Context())
	if err != nil {
		return err
	}

	applications, err := json.MarshalIndent(applicationList, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal application response as JSON %w", err)
	}
	fmt.Println(string(applications))

	return nil
}
