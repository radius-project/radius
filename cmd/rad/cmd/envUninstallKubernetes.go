// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/kubernetes/kubectl"
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
	err := kubectl.RunCLICommandSilent("delete", "gatewayclasses", "haproxy", "--ignore-not-found", "true")

	if err != nil {
		return err
	}

	var helmOutput strings.Builder
	helmConf, err := helm.HelmConfig(helm.RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	err = helm.RunDaprHelmUninstall(helmConf)
	if err != nil {
		return err
	}

	err = helm.RunHAProxyHelmUninstall(helmConf)
	if err != nil {
		return err
	}

	err = helm.RunRadiusHelmUninstall(helmConf)

	if err == nil {
		output.LogInfo("Finished uninstalling resources from namespace: %s", helm.RadiusSystemNamespace)
	}
	return err
}
