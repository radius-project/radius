// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/k3d"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/featureflag"
)

func init() {
	envInitCmd.AddCommand(envInitLocalCmd)

	// TODO: right now we only handle Azure as a special case. This needs to be generalized
	// to handle other providers.
	registerAzureProviderFlags(envInitLocalCmd)
	envInitLocalCmd.Flags().String("ucp-image", "", "Specify the UCP image to use")
	envInitLocalCmd.Flags().String("ucp-tag", "", "Specify the UCP tag to use")
}

type DevEnvironmentParams struct {
	Name      string
	Providers *environments.Providers
}

var envInitLocalCmd = &cobra.Command{
	Use:   "dev",
	Short: "Initializes a local development environment",
	Long:  `Initializes a local development environment`,
	RunE:  initDevRadEnvironment,
}

func initDevRadEnvironment(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	sharedArgs, err := parseArgs(cmd)
	if err != nil {
		return err
	}

	environmentName, err := selectEnvironment(cmd, "dev", sharedArgs.Interactive)
	if err != nil {
		return err
	}

	azureProvider, err := parseAzureProviderFromArgs(cmd, sharedArgs.Interactive)
	if err != nil {
		return err
	}

	params := &DevEnvironmentParams{
		Name:      environmentName,
		Providers: &environments.Providers{AzureProvider: azureProvider},
	}

	ucpImage, err := cmd.Flags().GetString("ucp-image")
	if err != nil {
		return err
	}

	ucpTag, err := cmd.Flags().GetString("ucp-tag")
	if err != nil {
		return err
	}

	_, foundConflict := env.Items[environmentName]
	if foundConflict {
		return fmt.Errorf("an environment named %s already exists. Use `rad env delete %s` to delete or select a different name", params.Name, params.Name)
	}

	// Create environment
	step := output.BeginStep("Creating Cluster...")
	cluster, err := k3d.CreateCluster(cmd.Context(), params.Name)
	if err != nil {
		return err
	}
	output.CompleteStep(step)

	step = output.BeginStep("Installing Radius...")

	client, runtimeClient, _, err := createKubernetesClients(cluster.ContextName)
	if err != nil {
		return err
	}

	cliOptions := helm.ClusterOptions{
		Namespace: sharedArgs.Namespace,
		Radius: helm.RadiusOptions{
			ChartPath:    sharedArgs.ChartPath,
			Image:        sharedArgs.Image,
			Tag:          sharedArgs.Tag,
			UCPImage:     ucpImage,
			UCPTag:       ucpTag,
			AppCoreImage: sharedArgs.AppCoreImage,
			AppCoreTag:   sharedArgs.AppCoreTag,
		},
	}
	options := helm.NewClusterOptions(cliOptions)
	options.Contour.HostNetwork = true
	options.Radius.PublicEndpointOverride = cluster.HTTPEndpoint
	options.Radius.AzureProvider = azureProvider

	err = helm.InstallOnCluster(cmd.Context(), options, client, runtimeClient)
	if err != nil {
		return err
	}

	// Persist settings
	env.Items[params.Name] = map[string]interface{}{
		"kind":        "dev",
		"context":     cluster.ContextName,
		"clustername": cluster.ClusterName,
		"namespace":   sharedArgs.Namespace,
		"registry": &environments.Registry{
			PushEndpoint: cluster.RegistryPushEndpoint,
			PullEndpoint: cluster.RegistryPullEndpoint,
		},
		"enableucp": featureflag.EnableUnifiedControlPlane.IsActive(),
	}

	if featureflag.EnableUnifiedControlPlane.IsActive() {
		// As decided by the team we will have a temporary 1:1 correspondence between UCP resource group and environment
		ucpRGName := fmt.Sprintf("%s-rg", environmentName)
		env.Items[params.Name]["ucpresourcegroupname"] = ucpRGName
		if createUCPResourceGroup(cluster.ContextName, ucpRGName) != nil {
			return err
		}
		if createEnvironmentResource(cluster.ContextName, ucpRGName, environmentName) != nil {
			return err
		}
	}

	output.CompleteStep(step)

	if params.Providers != nil {
		providerData := map[string]interface{}{}
		err = mapstructure.Decode(params.Providers, &providerData)
		if err != nil {
			return err
		}

		env.Items[params.Name]["providers"] = providerData
	}

	err = cli.SaveConfigOnLock(cmd.Context(), config, cli.UpdateEnvironmentWithLatestConfig(env, cli.MergeInitEnvConfig(params.Name)))
	if err != nil {
		return err
	}

	return nil
}
