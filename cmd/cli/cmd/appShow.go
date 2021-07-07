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
	config := ConfigFromContext(cmd.Context())
	env, err := rad.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	applicationName, err := rad.RequireApplicationArgs(cmd, args, env)
	if err != nil {
		return err
	}

	client, err := environments.CreateManagementClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	applicationResource, err := client.ShowApplication(cmd.Context(), applicationName)
	if err != nil {
		return err
	}

	format, err := rad.RequireOutput(cmd)
	if err != nil {
		return err
	}

	err = output.Write(format, applicationResource, cmd.OutOrStdout(), objectformats.GetApplicationTableFormat())
	if err != nil {
		return err
	}

	return nil
}
