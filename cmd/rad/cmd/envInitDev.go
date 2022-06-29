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

	// TODO: right now we only handle Azure as a special case. This needs to be generalized
	// to handle other providers.
	registerAzureProviderFlags(envInitLocalCmd)
	envInitLocalCmd.Flags().String("ucp-image", "", "Specify the UCP image to use")
	envInitLocalCmd.Flags().String("ucp-tag", "", "Specify the UCP tag to use")
}

type DevEnvironmentParams struct {
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
}
