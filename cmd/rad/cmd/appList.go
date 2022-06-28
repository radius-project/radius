// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/spf13/cobra"
)

// appListCmd command to list  applications deployed in the resource group
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
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	isUCPEnabled := false
	if env.GetKind() == environments.KindKubernetes {
		isUCPEnabled = env.(*environments.KubernetesEnvironment).GetEnableUCP()
	}
	if isUCPEnabled {
		err := listApplicationsUCP(cmd, args, env)
		if err != nil {
			return err
		}
	} else {
		err := listApplicationsLegacy(cmd, args, env)
		if err != nil {
			return err
		}
	}
	return nil
}

func listApplicationsLegacy(cmd *cobra.Command, args []string, env environments.Environment) error {
	client, err := environments.CreateLegacyManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	applicationList, err := client.ListApplications(cmd.Context())
	if err != nil {
		return err
	}

	return printOutput(cmd, applicationList.Value, true)
}

func listApplicationsUCP(cmd *cobra.Command, args []string, env environments.Environment) error {
	client, err := environments.CreateApplicationsManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}
	applicationList, err := client.ListApplications(cmd.Context())
	if err != nil {
		return err
	}

	return printOutput(cmd, applicationList, false)
}
