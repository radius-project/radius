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

var bicepCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean installed bicep compiler",
	Long:  `Removes the local copy of the bicep compiler`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger.LogInfo("removing local copy of bicep...")
		ok, err := bicep.IsBicepInstalled()
		if err != nil {
			return err
		}

		if !ok {
			logger.LogInfo("bicep is not installed")
			return err
		}

		err = bicep.CleanBicep()
		return err
	},
}

func init() {
	bicepCmd.AddCommand(bicepCleanCmd)
}
