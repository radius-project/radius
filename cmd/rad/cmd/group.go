// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(groupCmd)
}

func NewGroupCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "group",
		Short: "Manage resource groups",
		Long:  `Manage resource groups. This is NOT the same as Azure resource groups.`,
	}
}
