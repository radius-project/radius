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

	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli"
	resource_delete "github.com/project-radius/radius/pkg/cli/cmd/resource/delete"
	resource_list "github.com/project-radius/radius/pkg/cli/cmd/resource/list"
	resource_show "github.com/project-radius/radius/pkg/cli/cmd/resource/show"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
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

var resourceCmd = NewResourceCommand()

var ConfigHolderKey = NewContextKey("config")
var ConfigHolder = &framework.ConfigHolder{}

func prettyPrintRPError(err error) string {
	if new := clients.TryUnfoldErrorResponse(err); new != nil {
		m, err := prettyPrintJSON(new)
		if err == nil {
			return m
		}
	} else if new := clients.TryUnfoldServiceError(err); new != nil {
		m, err := prettyPrintJSON(new)
		if err == nil {
			return m
		}
	}

	return err.Error()
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
	ctx := context.WithValue(context.Background(), ConfigHolderKey, ConfigHolder)
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

	RootCmd.PersistentFlags().StringVar(&ConfigHolder.ConfigFilePath, "config", "", "config file (default \"$HOME/.rad/config.yaml\")")

	outputDescription := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	RootCmd.PersistentFlags().StringP("output", "o", output.DefaultFormat, outputDescription)
	initSubCommands()
}

func initSubCommands() {
	framework := &framework.Impl{
		ConnectionFactory: connections.DefaultFactory,
		ConfigHolder:      ConfigHolder,
		Output: &output.OutputWriter{
			Writer: RootCmd.OutOrStdout(),
		},
	}
	showCmd, _ := resource_show.NewCommand(framework)
	resourceCmd.AddCommand(showCmd)

	listCmd, _ := resource_list.NewCommand(framework)
	resourceCmd.AddCommand(listCmd)

	deleteCmd, _ := resource_delete.NewCommand(framework)
	resourceCmd.AddCommand(deleteCmd)
}

// The dance we do with config is kinda complex. We want commands to be able to retrieve a config (*viper.Viper)
// from context. However we need to initialize the context before we can read the config (before argument parsing).
//
// The solution is a double-indirection. We add a "ConfigHolder" to the context, and then initialize it later. This
// way the context is still immutable, but we can add the config when we're ready to (before any command runs).

func initConfig() {
	v, err := cli.LoadConfig(ConfigHolder.ConfigFilePath)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		os.Exit(1)
	}

	ConfigHolder.Config = v
}

type contextKey string

func NewContextKey(purpose string) contextKey {
	return contextKey("radius context " + purpose)
}

func ConfigFromContext(ctx context.Context) *viper.Viper {
	holder := ctx.Value(NewContextKey("config")).(*framework.ConfigHolder)
	if holder == nil {
		return nil
	}

	return holder.Config
}
