// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "Manage RAD resource groups",
	Long:  `Manage radius resource groups. This is NOT the same as azure resource group`,
}

func init() {
	RootCmd.AddCommand(groupCmd)
}
