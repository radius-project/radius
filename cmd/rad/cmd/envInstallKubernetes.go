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

	// Parameters to configure Azure provider for cloud resources
	envInstallKubernetesCmd.Flags().BoolP("provider-azure", "", false, "Add Azure provider for cloud resources")
	envInstallKubernetesCmd.Flags().String("provider-azure-subscription", "", "Azure subscription for cloud resources")
	envInstallKubernetesCmd.Flags().String("provider-azure-resource-group", "", "Azure resource-group for cloud resources")
	envInstallKubernetesCmd.Flags().StringP("provider-azure-client-id", "", "", "The client id for the service principal")
	envInstallKubernetesCmd.Flags().StringP("provider-azure-client-secret", "", "", "The client secret for the service principal")
	envInstallKubernetesCmd.Flags().StringP("provider-azure-tenant-id", "", "", "The tenant id for the service principal")
}
