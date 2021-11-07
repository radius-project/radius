// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/cli/server"
	"github.com/spf13/cobra"
)

var serverRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run local development server",
	Long:  `Run local development server provided by Radius`,
	RunE:  serverRun,
}

func init() {
	serverCmd.AddCommand(serverRunCmd)

	serverRunCmd.Flags().BoolP("clean", "c", false, "Clean server state")
}

func serverRun(cmd *cobra.Command, args []string) error {
	clean, err := cmd.Flags().GetBool("clean")
	if err != nil {
		return err
	}

	options := server.Options{
		Clean: clean,
	}

	err = server.Run(cmd.Context(), options)
	return err
}
