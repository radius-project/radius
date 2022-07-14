// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli/environments"
)

func init() {
	envInitCmd.AddCommand(envInitLocalCmd)
}

type EnvironmentParams struct {
	Name      string
	Providers *environments.Providers
}

var envInitLocalCmd = &cobra.Command{
	Use:   "dev",
	Short: "Initializes a local development environment",
	Long:  `Initializes a local development environment`,
	// RunE:  initDevRadEnvironment,
	RunE: func(cmd *cobra.Command, args []string) error {
		return initSelfHosted(cmd, args, Dev)
	},
	Hidden: true,
}
