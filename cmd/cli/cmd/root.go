// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"os"
	"path"

	"github.com/Azure/radius/pkg/version"
	"github.com/spf13/cobra"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var cfgFile string

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
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Initialize support for --version
	RootCmd.Version = version.Release()
	template := fmt.Sprintf("Release: %s \nVersion: %s\nCommit: %s\n", version.Release(), version.Version(), version.Commit())
	RootCmd.SetVersionTemplate(template)

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.rad/config.yaml)")
	RootCmd.PersistentFlags().StringP("application", "a", "", "The application name")
	RootCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
	RootCmd.PersistentFlags().StringP("component", "c", "", "The component name")
	RootCmd.PersistentFlags().StringP("deployment", "d", "", "The deployment name")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		rad := path.Join(home, ".rad")
		viper.AddConfigPath(rad)
		viper.SetConfigName("config")
	}

	// If a config file is found, read it in.
	err := viper.ReadInConfig()
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Set the default config file so we can write to it if desired
		cfgFile = path.Join(home, ".rad", "config.yaml")
	} else if err == nil {
		cfgFile = viper.ConfigFileUsed()
	}
}

func saveConfig() error {
	dir := path.Dir(cfgFile)
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		_ = os.MkdirAll(dir, os.ModeDir|0755)
	}

	err = viper.WriteConfigAs(cfgFile)
	if err != nil {
		return err
	}

	fmt.Printf("Successfully wrote configuration to %v\n", cfgFile)
	return nil
}

func require(cmd *cobra.Command, args []string, name string) (string, error) {
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", err
	}

	if len(args) > 0 {
		if args[0] != "" {
			if value != "" {
				return "", fmt.Errorf("cannot specify %v name via both arguments and `-d`", name)
			}
			value = args[0]
		}
	}

	if value == "" {
		return "", fmt.Errorf("no %v name provided", name)
	}

	return value, nil
}
