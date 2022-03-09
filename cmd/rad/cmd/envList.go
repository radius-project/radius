// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
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

	printFlag, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	var b []byte
	if(printFlag=="json") {
		b, err = json.MarshalIndent(&env, "", "  ")
	} else {
		b, err = yaml.Marshal(&env)
	}

	if err != nil {
		return err
	}
	fmt.Println(string(b))
	fmt.Println()
	return nil
}

func init() {
	envCmd.AddCommand(envListCmd)
}
