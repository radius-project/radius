// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

var envUninstallKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Uninstall Radius from Kubernetes Cluster",
	Long:  `Uninstall Radius from Kubernetes Cluster`,
	RunE:  envUninstallKubernetes,
}

func init() {
	envUninstallCmd.AddCommand(envUninstallKubernetesCmd)
}

func envUninstallKubernetes(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.RequireEnvironment(cmd, config)
	if err != nil {
		return err
	}

	err = helm.UninstallOnCluster(env.GetContext())
	if err != nil {
		return err
	}

	output.LogInfo("Finished uninstalling resources from namespace: %s", helm.RadiusSystemNamespace)
	return nil
}
