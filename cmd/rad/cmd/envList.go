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
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
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
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	isUCPEnabled := false
	if env.GetKind() == environments.KindKubernetes {
		isUCPEnabled = env.(*environments.KubernetesEnvironment).GetEnableUCP()
	}

	if isUCPEnabled {
		client, err := environments.CreateUCPManagementClient(cmd.Context(), env)
		if err != nil {
			return err
		}
		envList, err := client.ListEnv(cmd.Context())
		if err != nil {
			return err
		}
		return displayEnvListUCP(envList, cmd)
	} else {
		return displayEnvList(cmd)
	}
}

func displayEnvListUCP(envList []v20220315privatepreview.EnvironmentResource, cmd *cobra.Command) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	if format == "table" {
		err = output.Write(format, envList, cmd.OutOrStdout(), objectformats.GetGenericEnvironmentTableFormat())
		if err != nil {
			return err
		}
	} else if format == "json" {
		err = output.Write(format, envList, cmd.OutOrStdout(), output.FormatterOptions{Columns: []output.Column{}})
		if err != nil {
			return err
		}
	} else if format == "list" {
		b, err := yaml.Marshal(envList)
		if err != nil {
			return err
		}
		fmt.Println(string(b))

	} else {
		err = output.Write(format, envList, cmd.OutOrStdout(), objectformats.GetGenericEnvironmentTableFormat())
		if err != nil {
			return err
		}
	}

	return nil
}

func displayEnvList(cmd *cobra.Command) error {

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
		hasError, err := displayErrors(format, cmd, env)
		if err != nil {
			return err
		}
		if !hasError {
			err = output.Write(format, &env, cmd.OutOrStdout(), output.FormatterOptions{Columns: []output.Column{}})
			if err != nil {
				return err
			}
		}
	} else if format == "list" {
		b, err := yaml.Marshal(&env)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		_, err = displayErrors(format, cmd, env)
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

		_, err = displayErrors(format, cmd, env)
		if err != nil {
			return err
		}
	}
	return nil

}

func init() {
	envCmd.AddCommand(envListCmd)
}
