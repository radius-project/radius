// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
}

var envInitKubernetesCmd = &cobra.Command{
	Use:    "kubernetes",
	Short:  "Initializes a kubernetes environment",
	Long:   `Initializes a kubernetes environment.`,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return initSelfHosted(cmd, args, Kubernetes)
	},
}
