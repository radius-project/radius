// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(resourceCmd)
	resourceCmd.PersistentFlags().StringP("application", "a", "", "The application name")
	resourceCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}

func NewResourceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "resource",
		Short: "Manage resources",
		Long:  `Manage resources`,
	}
}
