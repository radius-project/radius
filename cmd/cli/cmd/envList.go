// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/radius/pkg/rad"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List environments",
	Long:  `List environments`,
	RunE: func(cmd *cobra.Command, args []string) error {

		config := ConfigFromContext(cmd.Context())
		env, err := rad.ReadEnvironmentSection(config)
		if err != nil {
			return err
		}

		if len(env.Items) == 0 {
			fmt.Println("No environments found. Use 'rad env init' to initialize.")
			return nil
		}

		b, err := yaml.Marshal(&env)
		if err != nil {
			return err
		}

		fmt.Println(string(b))
		fmt.Println()
		return nil
	},
}

func init() {
	envCmd.AddCommand(envListCmd)
}
