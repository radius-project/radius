// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/cli/bicep"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

var bicepDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete installed bicep compiler",
	Long:  `Removes the local copy of the bicep compiler`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.LogInfo("removing local copy of bicep...")
		ok, err := bicep.IsBicepInstalled()
		if err != nil {
			return err
		}

		if !ok {
			output.LogInfo("bicep is not installed")
			return err
		}

		err = bicep.DeleteBicep()
		return err
	},
}

func init() {
	bicepCmd.AddCommand(bicepDeleteCmd)
}
