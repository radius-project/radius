// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

const (
	CORE_RP_API_VERSION = "2022-03-15-privatepreview"
)

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
}

var envInitKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Initializes a kubernetes environment",
	Long:  `Initializes a kubernetes environment.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return initSelfHosted(cmd, args, Kubernetes)
	},
}
