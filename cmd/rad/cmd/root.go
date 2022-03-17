// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd is the root command of the rad CLI. This is exported so we can generate docs for it.
var RootCmd = &cobra.Command{
	Use:           "rad",
	Short:         "Project Radius CLI",
	Long:          `Project Radius CLI`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

const (
	UnlockErrorMessage string = "failed to unlock the config file"
)

func prettyPrintRPError(err error) string {
	raw := err.Error()
	if new := clients.TryUnfoldErrorResponse(err); new != nil {
		m, err := prettyPrintJSON(new)
		if err == nil {
			return m
		}
		return raw
	}
	return raw
}

func prettyPrintJSON(o interface{}) (string, error) {
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx := context.WithValue(context.Background(), configHolderKey, configHolder)
	err := RootCmd.ExecuteContext(ctx)
	if errors.Is(&cli.FriendlyError{}, err) {
		fmt.Println(err.Error())
		os.Exit(1)
	} else if err != nil {
		fmt.Println("Error:", prettyPrintRPError(err))
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(&configHolder.ConfigFilePath, "config", "", "config file (default \"$HOME/.rad/config.yaml\")")

	outputDescription := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	RootCmd.PersistentFlags().StringP("output", "o", output.DefaultFormat, outputDescription)
}

// The dance we do with config is kinda complex. We want commands to be able to retrieve a config (*viper.Viper)
// from context. However we need to initialize the context before we can read the config (before argument parsing).
//
// The solution is a double-indirection. We add a "ConfigHolder" to the context, and then initialize it later. This
// way the context is still immutable, but we can add the config when we're ready to (before any command runs).

type contextKey string

func NewContextKey(purpose string) contextKey {
	return contextKey("radius context " + purpose)
}

var configHolderKey = NewContextKey("config")
var configHolder = &ConfigHolder{}

type ConfigHolder struct {
	ConfigFilePath string
	Config         *viper.Viper
}

func ConfigFromContext(ctx context.Context) *viper.Viper {
	holder := ctx.Value(configHolderKey).(*ConfigHolder)
	if holder == nil {
		return nil
	}

	return holder.Config
}

func UpdateEnvironmentSectionOnCreation(environmentName string) func(*viper.Viper, cli.EnvironmentSection) error {
	return func(config *viper.Viper, env cli.EnvironmentSection) error {
		env.Default = environmentName
		output.LogInfo("Using environment: %v", environmentName)
		err := UpdateEnvironmentSection()(config, env)
		if err != nil {
			return err
		}
		return nil
	}
}

func UpdateEnvironmentSection() func(*viper.Viper, cli.EnvironmentSection) error {
	return func(config *viper.Viper, env cli.EnvironmentSection) error {
		cli.UpdateEnvironmentSection(config, env)
		return nil
	}
}

func SaveConfig(config *viper.Viper, env cli.EnvironmentSection, updateConfig func(*viper.Viper, cli.EnvironmentSection) error) error {

	latestConfig, err := cli.LoadConfig(configHolder.ConfigFilePath)
	if err != nil {
		return err
	}
	// Acquire exclusive lock on the config file.
	// Retry it every second for 5 times if other goroutine is holding the lock i.e other cmd is writing to the config file.
	configFilePath := cli.GetConfigFilePath(config)
	fileLock := flock.New(configFilePath)
	lockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = fileLock.TryLockContext(lockCtx, 1*time.Second)
	if err != nil {
		return fmt.Errorf("failed to acquire lock on '%s': %w", configFilePath, err)
	}

	updatedEnv, err := cli.ReadEnvironmentSection(latestConfig)
	if err != nil {
		if err := fileLock.Unlock(); err != nil {
			output.LogInfo(cli.UnlockErrorMessage)
		}
		return err
	}
	cli.MergeConfigs(env, updatedEnv)

	err = updateConfig(config, updatedEnv)
	if err != nil {
		if err := fileLock.Unlock(); err != nil {
			output.LogInfo(cli.UnlockErrorMessage)
		}
		return err
	}

	err = cli.SaveConfig(config)
	if err != nil {
		if err := fileLock.Unlock(); err != nil {
			output.LogInfo(cli.UnlockErrorMessage)
		}
		return err
	}

	if err := fileLock.Unlock(); err != nil {
		return fmt.Errorf("'%s': %w", cli.UnlockErrorMessage, err)
	}
	return nil

}

func initConfig() {
	v, err := cli.LoadConfig(configHolder.ConfigFilePath)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		os.Exit(1)
	}

	configHolder.Config = v
}
