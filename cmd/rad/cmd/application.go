// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(applicationCmd)
	applicationCmd.PersistentFlags().StringP("application", "a", "", "The application name")
	applicationCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}

func NewAppCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "application",
		Aliases: []string{"app"},
		Short:   "Manage RAD applications",
		Long:    `Manage RAD applications`,
	}
}
