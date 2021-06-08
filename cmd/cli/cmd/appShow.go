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

// appShowCmd command to show properties of an application
var appShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show RAD application details",
	Long:  "Show RAD application details",
	RunE:  showApplication,
}

func init() {
	applicationCmd.AddCommand(appShowCmd)
}

func showApplication(cmd *cobra.Command, args []string) error {
	env, err := rad.RequireEnvironment(cmd)
	if err != nil {
		return err
	}

	applicationName, err := rad.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(env)
	if err != nil {
		return err
	}

	applicationResource, err := client.ShowApplication(cmd.Context(), applicationName)
	if err != nil {
		return err
	}
	applicationDetails, err := json.MarshalIndent(applicationResource, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal application response as JSON %w", err)
	}
	fmt.Println(string(applicationDetails))

	return err
}
