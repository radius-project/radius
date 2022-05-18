// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments",
	Long:  `List environments`,
	RunE:  getEnvConfigs,
}

func getEnvConfigs(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	if len(env.Items) == 0 {
		fmt.Println("No environments found. Use 'rad env init' to initialize.")
		return nil
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	if format == "json" {
		env := populateEnvErrors(env)
		err = output.Write(format, &env, cmd.OutOrStdout(), output.FormatterOptions{Columns: []output.Column{}})
		if err != nil {
			return err
		}
	} else if format == "list" {
		b, err := yaml.Marshal(&env)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		err = displayErrors(cmd, env)
		if err != nil {
			return err
		}
	} else {
		//default format is table
		fmt.Println("default: " + env.Default)
		var envList []interface{}

		for key := range env.Items {
			env, _ := env.GetEnvironment(key)
			if env != nil {
				envList = append(envList, env)
			} else {
				undefinedEnv := &environments.GenericEnvironment{
					Name: key,
					Kind: "unknown",
				}
				envList = append(envList, undefinedEnv)
			}
		}

		formatter := objectformats.GetGenericEnvironmentTableFormat()
		err = output.Write(format, envList, cmd.OutOrStdout(), formatter)
		if err != nil {
			return err
		}

		err = displayErrors(cmd, env)
		if err != nil {
			return err
		}
	}
	return nil
}

func init() {
	envCmd.AddCommand(envListCmd)
}
