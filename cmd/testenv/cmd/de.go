// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli/de"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/version"
	"github.com/spf13/cobra"
)

var deCmd = &cobra.Command{
	Use:   "de",
	Short: "Manage Deployment Engine",
	Long:  `Manage Deployment Engine used by Radius for local testing`,
}

var deDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete installed Deployment Engine",
	Long:  `Removes the local copy of the Deployment Engine`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.LogInfo("removing local copy of Deployment Engine...")
		ok, err := de.IsDEInstalled()
		if err != nil {
			return err
		}

		if !ok {
			output.LogInfo("Deployment Engine is not installed")
			return err
		}

		err = de.DeleteDE()
		return err
	},
}

var deDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download the Deployment Engine",
	Long:  `Downloads the Deployment Engine locally`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.LogInfo(fmt.Sprintf("Downloading Deployment Engine for channel %s...", version.Channel()))
		err := de.DownloadDE()
		return err
	},
}

func init() {
	RootCmd.AddCommand(deCmd)
	deCmd.AddCommand(deDownloadCmd)
	deCmd.AddCommand(deDeleteCmd)
}
