// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad/bicep"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/version"
	"github.com/spf13/cobra"
)

var bicepDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download the bicep compiler",
	Long:  `Downloads the bicep compiler locally`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.LogInfo(fmt.Sprintf("Downloading Bicep for channel %s...", version.Channel()))
		err := bicep.DownloadBicep()
		return err
	},
}

func init() {
	bicepCmd.AddCommand(bicepDownloadCmd)
}
