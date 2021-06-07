// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"os"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/version"
	"github.com/spf13/cobra"
)

// RootCmd is the root command of the rad CLI. This is exported so we can generate docs for it.
var RootCmd = &cobra.Command{
	Use:           "rad",
	Short:         "Project Radius CLI",
	Long:          `Project Radius CLI`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Initialize support for --version
	RootCmd.Version = version.Release()
	template := fmt.Sprintf("Release: %s \nVersion: %s\nCommit: %s\n", version.Release(), version.Version(), version.Commit())
	RootCmd.SetVersionTemplate(template)

	RootCmd.Flags().BoolP("version", "v", false, "version for radius")
	RootCmd.PersistentFlags().StringVar(&rad.CfgFile, "config", "", "config file (default is $HOME/.rad/config.yaml)")
}

func initConfig() {
	rad.InitConfig()
}
