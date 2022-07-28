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
	setup.RegisterPersistantChartArgs(installKubernetesCmd)
	setup.RegistePersistantAzureProviderArgs(installKubernetesCmd)
}

func installKubernetes(cmd *cobra.Command, args []string) error {
	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	// It's ok if this is blank.
	kubeContext, err := cmd.Flags().GetString("kubecontext")
	if err != nil {
		return err
	}

	chartArgs, err := setup.ParseChartArgs(cmd)
	if err != nil {
		return err
	}

	// Configure Azure provider for cloud resources if specified
	azureProvider, err := setup.ParseAzureProviderArgs(cmd, interactive)
	if err != nil {
		return err
	}

	cliOptions := helm.CLIClusterOptions{
		Radius: helm.RadiusOptions{
			Reinstall:              chartArgs.Reinstall,
			ChartPath:              chartArgs.ChartPath,
			Image:                  chartArgs.Image,
			Tag:                    chartArgs.Tag,
			UCPImage:               chartArgs.UcpImage,
			UCPTag:                 chartArgs.UcpTag,
			AppCoreImage:           chartArgs.AppCoreImage,
			AppCoreTag:             chartArgs.AppCoreTag,
			PublicEndpointOverride: chartArgs.PublicEndpointOverride,
			AzureProvider:          azureProvider,
		},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	_, err = setup.Install(cmd.Context(), clusterOptions, kubeContext)
	if err != nil {
		return err
	}

	return nil
}
