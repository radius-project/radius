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

var envStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a local Radius environment",
	Long:  `Stop a local Radius environment. Uses the current user's default environment by default.`,
	RunE:  envStop,
}

func init() {
	envCmd.AddCommand(envStopCmd)
}

func envStop(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	lifecycle, err := environments.CreateServerLifecycleClient(cmd.Context(), env)
	if err != nil {
		return err
	}

	err = lifecycle.EnsureStopped(cmd.Context())
	if err != nil {
		return err
	}

	return nil
}
