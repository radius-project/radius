// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

var envStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show RAD environment status",
	Long:  "Show local Radius environment status. Uses the current user's default environment by default.",
	RunE:  envStatus,
}

func init() {
	envCmd.AddCommand(envStatusCmd)
}

func envStatus(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	lifecycle, err := environments.CreateServerLifecycleClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	status, err := lifecycle.GetStatus(cmd.Context())
	if err != nil {
		return err
	}

	output.LogInfo(status)
	return nil
}
