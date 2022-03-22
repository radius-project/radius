// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
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
	err := helm.UninstallOnCluster(cmd.Context())
	if err != nil {
		return err
	}

	output.LogInfo("Finished uninstalling resources from namespace: %s", helm.RadiusSystemNamespace)
	return nil
}
