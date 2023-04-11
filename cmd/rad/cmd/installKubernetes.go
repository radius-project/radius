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
	clikube "github.com/project-radius/radius/pkg/cli/kubernetes"
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
			Reinstall:              chartArgs.Reinstall,
			ChartPath:              chartArgs.ChartPath,
			UCPImage:               chartArgs.UcpImage,
			UCPTag:                 chartArgs.UcpTag,
			AppCoreImage:           chartArgs.AppCoreImage,
			AppCoreTag:             chartArgs.AppCoreTag,
			PublicEndpointOverride: chartArgs.PublicEndpointOverride,
			Values:                 chartArgs.Values,
		},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	alreadyInstalled, err := helm.Install(cmd.Context(), clusterOptions, kubeContext)
	if err != nil {
		return err
	}

	//installation completed. update workspaces, if any.
	if !alreadyInstalled {
		// install doesn't configure providers, user init to configure providers.
		err = updateWorkspaces(cmd.Context(), nil)
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

	currentContext, err := clikube.GetContextFromConfigFileIfExists("", "")
	if err != nil {
		return err
	}

	workspaceProvider := workspaces.AzureProvider{}
	if azProvider != nil {
		workspaceProvider = workspaces.AzureProvider{ResourceGroup: azProvider.ResourceGroup, SubscriptionID: azProvider.SubscriptionID}
	}
	err = cli.EditWorkspaces(ctx, config, func(section *cli.WorkspaceSection) error {
		cli.UpdateAzProvider(section, workspaceProvider, currentContext)
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
