// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/spf13/cobra"
)

var installKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Installs radius onto a kubernetes cluster",
	Long:  `Installs radius onto a kubernetes cluster`,
	RunE:  installKubernetes,
}

func init() {
	installCmd.AddCommand(installKubernetesCmd)
	installKubernetesCmd.PersistentFlags().BoolP("interactive", "i", false, "Collect values for required command arguments through command line interface prompts")
	installKubernetesCmd.Flags().String("kubecontext", "", "the Kubernetes context to use, will use the default if unset")
	setup.RegisterPersistentChartArgs(installKubernetesCmd)
}

func installKubernetes(cmd *cobra.Command, args []string) error {
	// It's ok if this is blank.
	kubeContext, err := cmd.Flags().GetString("kubecontext")
	if err != nil {
		return err
	}

	chartArgs, err := setup.ParseChartArgs(cmd)
	if err != nil {
		return err
	}

	cliOptions := helm.CLIClusterOptions{
		Radius: helm.RadiusOptions{
			Reinstall: chartArgs.Reinstall,
			ChartPath: chartArgs.ChartPath,
			Values:    chartArgs.Values,
		},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	_, err = helm.Install(cmd.Context(), clusterOptions, kubeContext)
	if err != nil {
		return err
	}

	return nil
}
