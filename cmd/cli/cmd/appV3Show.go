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

// appV3ShowCmd command to show properties of a V3 application
var appV3ShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show RADv3 application details",
	Long:  "Show RADv3 application details",
	RunE:  showApplicationV3,
}

func init() {
	applicationV3Cmd.AddCommand(appV3ShowCmd)
}

func showApplicationV3(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	applicationResource, err := client.ShowApplicationV3(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, applicationResource, cmd.OutOrStdout(), objectformats.GetApplicationTableFormat())
	if err != nil {
		return err
	}

	return nil
}
