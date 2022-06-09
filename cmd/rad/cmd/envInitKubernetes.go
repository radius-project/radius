// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/k3d"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
)

type envType string

const (
	k8s envType = "kubernetes"
	dev envType = "dev"
)

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
	registerAzureProviderFlags(envInitKubernetesCmd)
	envInitKubernetesCmd.Flags().String("ucp-image", "", "Specify the UCP image to use")
	envInitKubernetesCmd.Flags().String("ucp-tag", "", "Specify the UCP tag to use")
}

var envInitKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Initializes a kubernetes environment",
	Long:  `Initializes a kubernetes environment.`,
	RunE:  func(cmd *cobra.Command, args []string) error {
		return initStandalone(cmd, args, k8s)
	},
}

func createKubernetesClients(contextName string) (client_go.Interface, runtime_client.Client, string, error) {
	k8sConfig, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return nil, nil, "", err
	}

	if contextName == "" && k8sConfig.CurrentContext == "" {
		return nil, nil, "", errors.New("no kubernetes context is set")
	} else if contextName == "" {
		contextName = k8sConfig.CurrentContext
	}

	context := k8sConfig.Contexts[contextName]
	if context == nil {
		return nil, nil, "", fmt.Errorf("kubernetes context '%s' could not be found", contextName)
	}

	client, _, err := kubernetes.CreateTypedClient(contextName)
	if err != nil {
		return nil, nil, "", err
	}

	runtimeClient, err := kubernetes.CreateRuntimeClient(contextName, kubernetes.Scheme)
	if err != nil {
		return nil, nil, "", err
	}

	return client, runtimeClient, contextName, nil
}

func initStandalone(cmd *cobra.Command, args []string, t envType) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	sharedArgs, err := parseArgs(cmd)
	if err != nil {
		return err
	}

	ucpImage, err := cmd.Flags().GetString("ucp-image")
	if err != nil {
		return err
	}

	ucpTag, err := cmd.Flags().GetString("ucp-tag")
	if err != nil {
		return err
	}

	// Configure Azure provider for cloud resources if specified
	azureProvider, err := parseAzureProviderFromArgs(cmd, sharedArgs.Interactive)
	if err != nil {
		return err
	}

	if t == k8s {
		sharedArgs.Namespace, err = selectNamespace(cmd, "default", sharedArgs.Interactive)
		if err != nil {
			return err
		}
	}

	var defaultEnvName string

	switch t {
	case dev:
		defaultEnvName = "dev"
	case k8s:
		k8sConfig, err := kubernetes.ReadKubeConfig()
		if err != nil {
			return err
		}
		defaultEnvName = k8sConfig.CurrentContext

	default:
		return fmt.Errorf("unknown environment type: %s", t)
	}

	environmentName, err := selectEnvironment(cmd, defaultEnvName, sharedArgs.Interactive)
	if err != nil {
		return err
	}

	params := &EnvironmentParams{
		Name:      environmentName,
		Providers: &environments.Providers{AzureProvider: azureProvider},
	}

	_, foundConflict := env.Items[environmentName]
	if foundConflict {
		return fmt.Errorf("an environment named %s already exists. Use `rad env delete %s` to delete or select a different name", params.Name, params.Name)
	}

	clusterOptions := helm.ClusterOptions{
		Namespace: sharedArgs.Namespace,
		Radius: helm.RadiusOptions{
			ChartPath:     sharedArgs.ChartPath,
			Image:         sharedArgs.Image,
			Tag:           sharedArgs.Tag,
			UCPImage:      ucpImage,
			UCPTag:        ucpTag,
			AppCoreImage:  sharedArgs.AppCoreImage,
			AppCoreTag:    sharedArgs.AppCoreTag,
			AzureProvider: azureProvider,
		},
	}

	var k8sGoClient client_go.Interface
	var runtimeClient runtime_client.Client

	switch t {
	case dev:
		// Create environment
		step := output.BeginStep("Creating Cluster...")
		cluster, err := k3d.CreateCluster(cmd.Context(), params.Name)
		if err != nil {
			return err
		}
		output.CompleteStep(step)
		k8sGoClient, runtimeClient, _, err = createKubernetesClients(cluster.ContextName)
		if err != nil {
			return err
		}
		clusterOptions.Contour.HostNetwork = true
		clusterOptions.Radius.PublicEndpointOverride = cluster.HTTPEndpoint
		env.Items[params.Name] = map[string]interface{}{
			"kind":        "dev",
			"context":     cluster.ContextName,
			"clustername": cluster.ClusterName,
			"namespace":   sharedArgs.Namespace,
			"registry": &environments.Registry{
				PushEndpoint: cluster.RegistryPushEndpoint,
				PullEndpoint: cluster.RegistryPullEndpoint,
			},
		}

	case k8s:
		var contextName string
		k8sGoClient, runtimeClient, contextName, err = createKubernetesClients("")
		if err != nil {
			return err
		}
		env.Items[params.Name] = map[string]interface{}{
			"kind":      environments.KindKubernetes,
			"context":   contextName,
			"namespace": sharedArgs.Namespace,
		}

	}

	step := output.BeginStep("Installing Radius...")
	if err := helm.InstallOnCluster(cmd.Context(), clusterOptions, k8sGoClient, runtimeClient); err != nil {
		return err
	}
	output.CompleteStep(step)

	// Persist settings

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
