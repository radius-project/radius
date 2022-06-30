// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

var envInstallKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Installs radius onto a kubernetes cluster",
	Long:  `Installs radius onto a kubernetes cluster`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return initSelfHosted(cmd, args, Kubernetes)
	},
}

func init() {
	envInstallCmd.AddCommand(envInstallKubernetesCmd)
	envInstallKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInstallKubernetesCmd.Flags().StringP("namespace", "n", "default", "The namespace to use for the environment")
	envInstallKubernetesCmd.Flags().StringP("chart", "", "", "Specify a file path to a helm chart to install radius from")
	envInstallKubernetesCmd.Flags().String("image", "", "Specify the radius controller image to use")
	envInstallKubernetesCmd.Flags().String("tag", "", "Specify the radius controller tag to use")
	envInstallKubernetesCmd.Flags().String("appcore-image", "", "Specify Application.Core RP image to use")
	envInstallKubernetesCmd.Flags().String("appcore-tag", "", "Specify Application.Core RP image tag to use")
	envInstallKubernetesCmd.Flags().String("ucp-image", "", "Specify the UCP image to use")
	envInstallKubernetesCmd.Flags().String("ucp-tag", "", "Specify the UCP tag to use")

	// Parameters to configure Azure provider for cloud resources
	registerAzureProviderFlags(envInstallKubernetesCmd)
}
