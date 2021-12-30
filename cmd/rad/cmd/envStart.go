// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/spf13/cobra"
)

var envStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Show RAD environment details",
	Long:  `Start a local RAD environment.  Uses the current user's default environment by default.`,
	RunE:  envStart,
}

func init() {
	envCmd.AddCommand(envStartCmd)
}

func envStart(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	lifecycle, err := environments.CreateServerLifecycleClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	err = lifecycle.EnsureStarted(cmd.Context())
	if err != nil {
		return err
	}

	return nil
}
