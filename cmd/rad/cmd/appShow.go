// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

// appShowCmd command to show properties of a  application
var appShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show RAD application details",
	Long:  "Show RAD application details",
	RunE:  showApplication,
}

func init() {
	applicationCmd.AddCommand(appShowCmd)
}

func showApplication(cmd *cobra.Command, args []string) error {
	return nil
}
