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
		fmt.Println("Default: "+env.Default)
			for key := range env.Items {
			e,err := env.GetEnvironment(key)
			if err != nil {
				return err
			}
			formatter := objectformats.GetGenericEnvironmentTableFormat()
			if e.GetKind() == environments.KindAzureCloud {
				formatter = objectformats.GetAzureCloudEnvironmentTableFormat()
			} else if e.GetKind() == environments.KindDev {
				formatter = objectformats.GetLocalEnvironmentTableFormat()
			} else if e.GetKind() == environments.KindKubernetes {
				formatter = objectformats.GetKubernetesEnvironmentTableFormat()
			} else if e.GetKind() == environments.KindLocalRP {
				formatter = objectformats.GetLocalRpTableEnvironmentFormat()
			}
			err = output.Write(format, e, cmd.OutOrStdout(), formatter)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func init() {
	envCmd.AddCommand(envListCmd)
}
