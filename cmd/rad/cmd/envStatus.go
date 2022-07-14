// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

var envStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show local Radius environment status",
	Long:  "Show local Radius environment status. Uses the current user's default environment by default.",
	RunE:  envStatus,
}

func init() {
	envCmd.AddCommand(envStatusCmd)
}

func envStatus(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	lifecycle, err := connections.DefaultFactory.CreateServerLifecycleClient(cmd.Context(), *workspace)
	if err != nil {
		return err
	}

	status, columns, err := lifecycle.GetStatus(cmd.Context())
	if err != nil {
		return err
	}

	err = output.Write(format, status, cmd.OutOrStdout(), output.FormatterOptions{Columns: columns})
	if err != nil {
		return err
	}

	return nil
}
