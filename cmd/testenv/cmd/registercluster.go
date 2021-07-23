// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/radius/pkg/rad"
	"github.com/spf13/cobra"
)

var registerClusterCmd = &cobra.Command{
	Use:   "registercluster",
	Short: "Updates rad environment config with provided kubernetes cluster name",
	Long:  "Updates rad environment config with provided kubernetes cluster name",
	RunE: func(cmd *cobra.Command, args []string) error {
		configpath, err := cmd.Flags().GetString("configpath")
		if err != nil {
			return err
		}

		clustername, err := cmd.Flags().GetString("clustername")
		if err != nil {
			return err
		}

		namespace, err := cmd.Flags().GetString("namespace")
		if err != nil {
			return err
		}

		v, err := rad.LoadConfig(configpath)
		if err != nil {
			return err
		}

		env, err := rad.ReadEnvironmentSection(v)
		if err != nil {
			return err
		}

		env.Default = clustername
		env.Items[clustername] = map[string]interface{}{
			"kind":      "kubernetes",
			"context":   clustername,
			"namespace": namespace,
		}

		rad.UpdateEnvironmentSection(v, env)
		err = rad.SaveConfig(v)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(registerClusterCmd)
	registerClusterCmd.Flags().StringP("configpath", "t", "", "specifies location to write config")
	registerClusterCmd.Flags().StringP("clustername", "c", "", "specifies the kubernetes clustername")
	registerClusterCmd.Flags().StringP("namespace", "n", "", "specifies the namespace for the environment")
}
