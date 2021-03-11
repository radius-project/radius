// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/rad/bicep"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/spf13/cobra"
)

var bicepDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download the bicep compiler",
	Long:  `Downloads the bicep compiler locally`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.LogInfo("downloading bicep...")
		err := bicep.DownloadBicep()
		return err
	},
}

func init() {
	bicepCmd.AddCommand(bicepDownloadCmd)
}
