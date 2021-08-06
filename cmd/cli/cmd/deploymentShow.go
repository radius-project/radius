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

// deploymentShowCmd command to show details of a deployment
var deploymentShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show Radius deployment details",
	Long:  "Show details of the specified Radius deployment deployed in the default environment",
	RunE:  showDeployment,
}

func init() {
	deploymentCmd.AddCommand(deploymentShowCmd)
}

func showDeployment(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := cli.RequireApplication(cmd, env)
	if err != nil {
		return err
	}

	deploymentName, err := cli.RequireDeployment(cmd, args)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	deploymentResource, err := client.ShowDeployment(cmd.Context(), applicationName, deploymentName)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, deploymentResource, cmd.OutOrStdout(), objectformats.GetDeploymentTableFormat())
	if err != nil {
		return err
	}

	return nil
}
