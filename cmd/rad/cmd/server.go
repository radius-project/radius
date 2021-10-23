// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage local development server",
	Long:  `Manage local development server used by Radius`,
}

func init() {
	RootCmd.AddCommand(serverCmd)
}
