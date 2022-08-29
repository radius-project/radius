// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/spf13/cobra"
)

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments",
	Long:  `List environments`,
	RunE:  getEnvConfigs,
}

func getEnvConfigs(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}
	envList, err := client.ListEnv(cmd.Context())
	if err != nil {
		return err
	}
	return displayEnvList(envList, cmd)

}

func displayEnvList(envList []v20220315privatepreview.EnvironmentResource, cmd *cobra.Command) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, envList, cmd.OutOrStdout(), objectformats.GetGenericEnvironmentTableFormat())
	if err != nil {
		return err
	}
	return nil
}

func init() {
	envCmd.AddCommand(envListCmd)
}
