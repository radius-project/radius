// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

var appStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Radius application status",
	Long:  "Show Radius application status",
	RunE:  showApplicationStatus,
}

func init() {
	applicationCmd.AddCommand(appStatusCmd)
}

func showApplicationStatus(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateLegacyManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	applicationStatus, err := client.ShowApplicationStatus(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, applicationStatus, cmd.OutOrStdout(), objectformats.GetApplicationStatusTableFormatOld())
	if err != nil {
		return err
	}

	if format == output.FormatTable && len(applicationStatus.Gateways) > 0 {
		err = output.Write(format, applicationStatus.Gateways, cmd.OutOrStdout(), objectformats.GetApplicationGatewaysTableFormatOld())
		if err != nil {
			return err
		}
	}

	return nil
}
