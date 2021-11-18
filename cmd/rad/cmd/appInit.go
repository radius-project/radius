// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"os"
	"path"

	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/scaffold"
	"github.com/spf13/cobra"
)

// appInitCmd command to scaffold
var appInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold RAD application",
	Long:  "Scaffolds a starter RAD application in the current directory",
	Args:  cobra.MaximumNArgs(1),
	RunE:  initApplication,
}

func init() {
	applicationCmd.AddCommand(appInitCmd)
}

func initApplication(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// We're avoiding the validation functionality in the CLI package intentionally
	// because the init command has different semantics.
	//
	// - We don't require an application (default is working directory)
	// - We don't want to use the "default application" in the config file, it would just be surprising.
	applicationName, err := cmd.Flags().GetString("application")
	if err != nil {
		return err
	}

	if len(args) > 0 && applicationName != "" {
		return fmt.Errorf("cannot specify application name via both arguments and `-a`")
	} else if applicationName != "" {
		// Do nothing
	} else if len(args) > 0 {
		applicationName = args[0]
	} else {
		applicationName = path.Base(wd)
	}

	output.LogInfo("Initializing Application %s...", applicationName)
	files, err := scaffold.WriteApplication(wd, applicationName)
	if err != nil {
		return err
	}

	output.LogInfo("")
	for _, file := range files {
		output.LogInfo("\tCreated %s", file)
	}
	output.LogInfo("")

	output.LogInfo("Have a RAD time 🕶️")
	return nil
}
