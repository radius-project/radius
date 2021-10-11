// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"os"

	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/scaffold"
	"github.com/spf13/cobra"
)

// appInitCmd command to scaffold
var appInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold RAD application",
	Long:  "Scaffolds a starter RAD application in the current directory",
	Args:  cobra.ExactArgs(0),
	RunE:  initApplication,
}

func init() {
	applicationCmd.AddCommand(appInitCmd)
}

func initApplication(cmd *cobra.Command, args []string) error {
	output.LogInfo("Creating Application...")

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	err = scaffold.WriteApplication(wd)
	if err != nil {
		return err
	}

	output.LogInfo("")
	output.LogInfo("\tCreated %s", "infra.bicep")
	output.LogInfo("\tCreated %s", "app.bicep")
	output.LogInfo("\tCreated %s", "rad.yaml")
	output.LogInfo("")

	output.LogInfo("Have a RAD time 🕶️")
	return nil
}
