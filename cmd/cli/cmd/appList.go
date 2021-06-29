// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/objectformats"
	"github.com/Azure/radius/pkg/rad/output"
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

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	applicationList, err := client.ListApplications(cmd.Context())
	if err != nil {
		return err
	}

	format, err := rad.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, applicationList.Value, cmd.OutOrStdout(), objectformats.GetApplicationTableFormat())
	if err != nil {
		return err
	}

	return nil
}
