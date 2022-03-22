// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments",
	Long:  `List environments`,
	RunE: getEnvConfigs,
}

func getEnvConfigs(cmd *cobra.Command, args [] string) error {
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
		fmt.Println()
	} else {//default format is table
		fmt.Println("default: "+env.Default)
		var eList []interface{}
		for key := range env.Items {
			e,err := env.GetEnvironment(key)
			eList = append(eList, e)
			if err != nil {
				return err
			}
		}
		formatter := objectformats.GetGenericEnvironmentTableFormat()
		err = output.Write(format, eList, cmd.OutOrStdout(), formatter)
		if err != nil {
			return err
		}
	}
	return nil
}

func init() {
	envCmd.AddCommand(envListCmd)
}
