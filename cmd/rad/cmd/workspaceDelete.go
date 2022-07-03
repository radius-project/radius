// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/spf13/cobra"
)

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Uninstall radius for a specific platform",
	Long:  `Uninstall radius for a specific platform`,
	RunE:  workspaceDelete,
}

func init() {
	workspaceCmd.AddCommand(workspaceDeleteCmd)
}

func workspaceDelete(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	// Delete env from the config, update default env if needed
	if err = deleteEnvFromConfig(cmd.Context(), config, env.GetName()); err != nil {
		return err
	}

	return nil
}
