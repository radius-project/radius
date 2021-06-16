// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/rad"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var envInitLocalCmd = &cobra.Command{
	Use:   "local",
	Short: "Initializes a local environment",
	Long:  `Initializes a local environment`,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()
		env, err := rad.ReadEnvironmentSection(v)
		if err != nil {
			return err
		}

		env.Items["local"] = map[string]interface{}{
			"kind": "local",
		}
		if len(env.Items) == 1 {
			env.Default = "local"
		}
		rad.UpdateEnvironmentSection(v, env)

		err = rad.SaveConfig()
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	envInitCmd.AddCommand(envInitLocalCmd)
}
