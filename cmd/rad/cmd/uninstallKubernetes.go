// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/spf13/cobra"
)

var uninstallKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Uninstall Radius from Kubernetes Cluster",
	Long:  `Uninstall Radius from Kubernetes Cluster`,
	RunE:  uninstallKubernetes,
}

func init() {
	uninstallCmd.AddCommand(uninstallKubernetesCmd)

	uninstallKubernetesCmd.Flags().String("kubecontext", "", "the Kubernetes context to use, will use the default if unset")
}

func uninstallKubernetes(cmd *cobra.Command, args []string) error {
	kubeContext, err := cmd.Flags().GetString("kubecontext")
	if err != nil {
		return err
	}

	// It's OK for KubeContext to be blank.
	err = setup.Uninstall(cmd.Context(), kubeContext)
	if err != nil {
		return err
	}

	return nil
}
