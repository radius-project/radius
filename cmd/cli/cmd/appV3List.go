// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/objectformats"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

// appV3ListCmd command to list V3 applications deployed in the resource group
var appV3ListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists RADv3 applications",
	Long:  "Lists RADv3 applications deployed in the resource group associated with the default environment",
	Args:  cobra.ExactArgs(0),
	RunE:  listApplicationsV3,
}

func init() {
	applicationV3Cmd.AddCommand(appV3ListCmd)
}

func listApplicationsV3(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	applicationList, err := client.ListApplicationsV3(cmd.Context())
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, applicationList.Value, cmd.OutOrStdout(), objectformats.GetApplicationTableFormat())
	if err != nil {
		return err
	}

	return nil
}
