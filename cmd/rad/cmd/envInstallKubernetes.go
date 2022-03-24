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
	RunE:  installKubernetes,
}

func init() {
	envInstallCmd.AddCommand(envInstallKubernetesCmd)
	envInstallKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInstallKubernetesCmd.Flags().StringP("namespace", "n", "default", "The namespace to use for the environment")
	envInstallKubernetesCmd.Flags().StringP("chart", "", "", "Specify a file path to a helm chart to install radius from")
	envInstallKubernetesCmd.Flags().String("image", "", "Specify the radius controller image to use")
	envInstallKubernetesCmd.Flags().String("tag", "", "Specify the radius controller tag to use")
}
