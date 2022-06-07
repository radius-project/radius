// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
)

var envInitKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Initializes a kubernetes environment",
	Long:  `Initializes a kubernetes environment.`,
	RunE:  installKubernetes,
}

func installKubernetes(cmd *cobra.Command, args []string) error {
	sharedArgs, err := parseArgs(cmd)
	if err != nil {
		return err
	}
	sharedArgs.Namespace, err = selectNamespace(cmd, "default", sharedArgs.Interactive)
	if err != nil {
		return err
	}

	client, runtimeClient, contextName, err := createKubernetesClients("")
	if err != nil {
		return err
	}
	environmentName, err := selectEnvironment(cmd, contextName, sharedArgs.Interactive)
	if err != nil {
		return err
	}

	// Configure Azure provider for cloud resources if specified
	azureProvider, err := parseAzureProviderFromArgs(cmd, sharedArgs.Interactive)
	ucpImage, err := cmd.Flags().GetString("ucp-image")
	if err != nil {
		return err
	}

	ucpTag, err := cmd.Flags().GetString("ucp-tag")
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}

	config := ConfigFromContext(cmd.Context())
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	_, foundConflict := env.Items[environmentName]
	if foundConflict {
		return fmt.Errorf("an environment named %s already exists. Use `rad env delete %s` to delete or select a different name", environmentName, environmentName)
	}

	step := output.BeginStep("Installing Radius...")

	cliOptions := helm.ClusterOptions{
		Namespace: sharedArgs.Namespace,
		Radius: helm.RadiusOptions{
			ChartPath: sharedArgs.ChartPath,
			Image:     sharedArgs.Image,
			Tag:       sharedArgs.Tag,
			UCPImage:  ucpImage,
			UCPTag:    ucpTag,
		},
	}

	clusterOptions := helm.NewClusterOptions(cliOptions)
	clusterOptions.Radius.AzureProvider = azureProvider

	if err := helm.InstallOnCluster(cmd.Context(), clusterOptions, client, runtimeClient); err != nil {
		return err
	}

	output.CompleteStep(step)

	env.Items[environmentName] = map[string]interface{}{
		"kind":      environments.KindKubernetes,
		"context":   contextName,
		"namespace": sharedArgs.Namespace,
	}

	if err := cli.SaveConfigOnLock(cmd.Context(), config, cli.UpdateEnvironmentWithLatestConfig(env, cli.MergeInitEnvConfig(environmentName))); err != nil {
		return err
	}

	return nil
}

func createKubernetesClients(contextName string) (client_go.Interface, runtime_client.Client, string, error) {
	k8sconfig, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return nil, nil, "", err
	}

	if contextName == "" && k8sconfig.CurrentContext == "" {
		return nil, nil, "", errors.New("no kubernetes context is set")
	} else if contextName == "" {
		contextName = k8sconfig.CurrentContext
	}

	context := k8sconfig.Contexts[contextName]
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

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
	registerAzureProviderFlags(envInitKubernetesCmd)
	envInitKubernetesCmd.Flags().String("ucp-image", "", "Specify the UCP image to use")
	envInitKubernetesCmd.Flags().String("ucp-tag", "", "Specify the UCP tag to use")
}
