// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/featureflag"
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
			ChartPath:    sharedArgs.ChartPath,
			Image:        sharedArgs.Image,
			Tag:          sharedArgs.Tag,
			UCPImage:     ucpImage,
			UCPTag:       ucpTag,
			AppCoreImage: sharedArgs.AppCoreImage,
			AppCoreTag:   sharedArgs.AppCoreTag,
		},
	}

	clusterOptions := helm.NewClusterOptions(cliOptions)
	clusterOptions.Radius.AzureProvider = azureProvider

	if err := helm.InstallOnCluster(cmd.Context(), clusterOptions, client, runtimeClient); err != nil {
		return err
	}

	env.Items[environmentName] = map[string]interface{}{
		"kind":      environments.KindKubernetes,
		"context":   contextName,
		"namespace": sharedArgs.Namespace,
		"enableucp": featureflag.EnableUnifiedControlPlane.IsActive(),
	}

	if featureflag.EnableUnifiedControlPlane.IsActive() {
		rgName := fmt.Sprintf("%s-rg", environmentName)
		env.Items[environmentName]["ucpresourcegroupname"] = rgName
		if createUCPResourceGroup(contextName, rgName) != nil {
			return err
		}
		if createEnvironmentResource(contextName, rgName, environmentName) != nil {
			return err
		}
	}

	output.CompleteStep(step)

	if err := cli.SaveConfigOnLock(cmd.Context(), config, cli.UpdateEnvironmentWithLatestConfig(env, cli.MergeInitEnvConfig(environmentName))); err != nil {
		return err
	}

	return nil
}

func createUCPResourceGroup(kubeCtxName, resourceGroupName string) error {
	baseUrl, rt, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(
		"",
		"",
		kubeCtxName,
		featureflag.EnableUnifiedControlPlane.IsActive(),
	)
	if err != nil {
		return err
	}

	createRgRequest, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/planes/radius/local/resourceGroups/%s", baseUrl, resourceGroupName),
		strings.NewReader(`{}`),
	)
	if err != nil {
		return fmt.Errorf("failed to create UCP resourceGroup: %w", err)
	}
	res, err := rt.RoundTrip(createRgRequest)
	if err != nil {
		return fmt.Errorf("failed to create UCP resourceGroup: %w", err)
	}

	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("request to create UCP resouceGroup failed with status: %d, request: %+v", res.StatusCode, res)
	}
	return nil
}

func createEnvironmentResource(kubeCtxName, resourceGroupName, environmentName string) error {
	baseUrl, rt, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(
		"",
		"",
		kubeCtxName,
		featureflag.EnableUnifiedControlPlane.IsActive(),
	)
	if err != nil {
		return err
	}

	createRgRequest, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s?api-version=2022-03-15-privatepreview", baseUrl, resourceGroupName, environmentName),
		strings.NewReader(`{"properties":{"compute":{"kind":""}}}`),
	)
	createRgRequest.Header.Add("Content-Type", "application/json")
	if err != nil {
		return fmt.Errorf("failed to create Applications.Core/environments resource: %w", err)
	}
	res, err := rt.RoundTrip(createRgRequest)
	if err != nil {
		return fmt.Errorf("failed to create Applications.Core/environments resource: %w", err)
	}

	if res.StatusCode != http.StatusOK { //shouldn't it be http.StatusCreated for consistency with rg?
		return fmt.Errorf("request to create Applications.Core/environments resource failed with status: %d, request: %+v", res.StatusCode, res)
	}
	return nil
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

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
	registerAzureProviderFlags(envInitKubernetesCmd)
	envInitKubernetesCmd.Flags().String("ucp-image", "", "Specify the UCP image to use")
	envInitKubernetesCmd.Flags().String("ucp-tag", "", "Specify the UCP tag to use")
}
