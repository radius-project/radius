// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
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

	alreadyInstalled, err := setup.Install(cmd.Context(), clusterOptions, kubeContext)
	if err != nil {
		return err
	}

	//installation completed. update workspaces, if any.
	if !alreadyInstalled {
		err = updateWorkspaces(cmd.Context(), azureProvider)
		if err != nil {
			return err
		}
	}

	return nil
}

func updateWorkspaces(ctx context.Context, azProvider *azure.Provider) error {

	config := ConfigFromContext(ctx)
	section, err := cli.ReadWorkspaceSection(config)
	if err != nil {
		return err
	}
	if len(section.Items) == 0 {
		return nil
	}

	currentKubeContext, err := getCurrentKubeContext()
	if err != nil {
		return err
	}
	err = cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {

		for _, workspaceItem := range section.Items {
			if workspaceItem.IsSameKubernetesContext(currentKubeContext) {
				workspaceName := workspaceItem.Name
				if azProvider == nil {
					workspaceItem.ProviderConfig.Azure = &workspaces.AzureProvider{}
				} else {
					workspaceItem.ProviderConfig.Azure = &workspaces.AzureProvider{ResourceGroup: azProvider.ResourceGroup, SubscriptionID: azProvider.SubscriptionID}
				}
				section.Items[workspaceName] = workspaceItem
			}

		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func getCurrentKubeContext() (string, error) {
	k8sConfig, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return "", err
	}

	return k8sConfig.CurrentContext, nil
}
