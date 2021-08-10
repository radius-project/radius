// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/bicep"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/Azure/radius/pkg/version"
	"github.com/ghodss/yaml"
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

// prettyPrintRPError prints an error that wraps a
// *radclient.ErrorResponse as YAML.
//
// YAML is a good format for this purpose because of the `|` string
// quoting syntax that allows nested YAML strings to have good
// readability.
//
// In case we don't understand the error format we are dealing with,
// we will just return "".
func prettyPrintRPError(err error) string {
	inner, ok := errors.Unwrap(err).(*radclient.ErrorResponse)
	if inner == nil || !ok {
		return ""
	}
	// Now, attempt to reformat the message too
	for i := range inner.InnerError.Details {
		if inner.InnerError.Details[i] == nil || inner.InnerError.Details[i].Message == nil {
			continue
		}
		y, err := yaml.JSONToYAML([]byte(*inner.InnerError.Details[i].Message))
		if err != nil {
			continue
		}
		s := string(y)
		inner.InnerError.Details[i].Message = &s
	}
	if b, err := yaml.Marshal(inner.InnerError); err == nil {
		return string(b)
	}
	return ""
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx := context.WithValue(context.Background(), configHolderKey, configHolder)
	if err := RootCmd.ExecuteContext(ctx); err != nil {
		if yaml := prettyPrintRPError(err); yaml != "" {
			fmt.Printf("Error from Radius resource provider:\n%s\n", yaml)
		} else {
			fmt.Println("Error:", err)
		}
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(func() {
		v, err := cli.LoadConfig(configHolder.ConfigFilePath)
		if err != nil {
			fmt.Printf("Error: failed to load config: %v\n", err)
			os.Exit(1)
		}

		configHolder.Config = v
	})

	// Initialize support for --version
	RootCmd.Version = version.Release()
	template := fmt.Sprintf("Release: %s \nVersion: %s\nBicep version: %s\nCommit: %s\n", version.Release(), version.Version(), bicep.Version(), version.Commit())
	RootCmd.SetVersionTemplate(template)

	RootCmd.Flags().BoolP("version", "v", false, "version for radius")
	RootCmd.PersistentFlags().StringVar(&configHolder.ConfigFilePath, "config", "", "config file (default is $HOME/.rad/config.yaml)")

	outputDescription := fmt.Sprintf("output format (default is %s, supported formats are %s)", output.DefaultFormat, strings.Join(output.SupportedFormats(), ", "))
	RootCmd.PersistentFlags().StringP("output", "o", "table", outputDescription)
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
