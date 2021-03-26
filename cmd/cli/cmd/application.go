// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var applicationCmd = &cobra.Command{
	Use:   "application",
	Short: "Manage applications",
	Long:  `Manage applications`,
}

func init() {
	rootCmd.AddCommand(applicationCmd)
}
