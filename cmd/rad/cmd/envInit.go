// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
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
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	coreRpApps "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

var envInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a RAD environment",
	Long:  `Create a RAD environment`,
}

func init() {
	envCmd.AddCommand(envInitCmd)
	envInitCmd.PersistentFlags().BoolP("interactive", "i", false, "Collect values for required command arguments through command line interface prompts")
	envInitCmd.PersistentFlags().StringP("namespace", "n", "default", "Specify the namespace to use for the environment into which application resources are deployed")
	setup.RegisterPersistantChartArgs(envInitCmd)
	setup.RegistePersistantAzureProviderArgs(envInitCmd)
}

type EnvKind int

const (
	Kubernetes EnvKind = iota
	Dev
)

func (k EnvKind) String() string {
	return [...]string{"Kubernetes", "Dev"}[k]
}

func initSelfHosted(cmd *cobra.Command, args []string, kind EnvKind) error {
	config := ConfigFromContext(cmd.Context())
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	namespace, err := selectNamespace(cmd, "default", interactive)
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

	var defaultEnvName string

	switch kind {
	case Dev:
		defaultEnvName = "dev"
	case Kubernetes:
		k8sConfig, err := kubernetes.ReadKubeConfig()
		if err != nil {
			return err
		}
		defaultEnvName = k8sConfig.CurrentContext

	default:
		return fmt.Errorf("unknown environment type: %s", kind)
	}

	environmentName, err := selectEnvironment(cmd, defaultEnvName, interactive)
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

	cliOptions := helm.CLIClusterOptions{
		Radius: helm.RadiusOptions{
			Reinstall:     chartArgs.Reinstall,
			ChartPath:     chartArgs.ChartPath,
			Image:         chartArgs.Image,
			Tag:           chartArgs.Tag,
			UCPImage:      chartArgs.UcpImage,
			UCPTag:        chartArgs.UcpTag,
			AppCoreImage:  chartArgs.AppCoreImage,
			AppCoreTag:    chartArgs.AppCoreTag,
			AzureProvider: azureProvider,
		},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	var k8sGoClient client_go.Interface
	var contextName string
	switch kind {
	case Dev:
		// Create environment
		step := output.BeginStep("Creating Cluster...")
		cluster, err := k3d.CreateCluster(cmd.Context(), params.Name)
		if err != nil {
			return err
		}
		output.CompleteStep(step)
		k8sGoClient, _, _, err = createKubernetesClients(cluster.ContextName)
		if err != nil {
			return err
		}
		clusterOptions.Contour.HostNetwork = true
		clusterOptions.Radius.PublicEndpointOverride = cluster.HTTPEndpoint
		env.Items[params.Name] = map[string]interface{}{
			"kind":        "dev",
			"context":     cluster.ContextName,
			"clustername": cluster.ClusterName,
			"namespace":   namespace,
			"registry": &environments.Registry{
				PushEndpoint: cluster.RegistryPushEndpoint,
				PullEndpoint: cluster.RegistryPullEndpoint,
			},
		}

	case Kubernetes:
		k8sGoClient, _, contextName, err = createKubernetesClients("")
		if err != nil {
			return err
		}
		env.Items[params.Name] = map[string]interface{}{
			"kind":      environments.KindKubernetes,
			"context":   contextName,
			"namespace": namespace,
		}
	}

	// Make sure namespace for applications exists
	err = kubernetes.EnsureNamespace(cmd.Context(), k8sGoClient, namespace)
	if err != nil {
		return err
	}

	err = setup.Install(cmd.Context(), clusterOptions, contextName)
	if err != nil {
		return err
	}

	step := output.BeginStep("Creating Environment...")

	// As decided by the team we will have a temporary 1:1 correspondence between UCP resource group and environment
	ucpRgName := fmt.Sprintf("%s-rg", environmentName)
	env.Items[environmentName]["ucpresourcegroupname"] = ucpRgName
	ucpRgId, err := createUCPResourceGroup(contextName, ucpRgName, "/planes/radius/local")
	if err != nil {
		return err
	}

	_, err = createUCPResourceGroup(contextName, ucpRgName, "/planes/deployments/local")
	if err != nil {
		return err
	}

	env.Items[environmentName]["scope"] = ucpRgId
	ucpEnvId, err := createEnvironmentResource(cmd.Context(), contextName, ucpRgName, environmentName)
	if err != nil {
		return err
	}
	env.Items[environmentName]["id"] = ucpEnvId

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

func createUCPResourceGroup(kubeCtxName, resourceGroupName string, plane string) (string, error) {
	baseUrl, rt, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(
		"",
		"",
		kubeCtxName,
	)
	if err != nil {
		return "", err
	}

	createRgRequest, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s%s/resourceGroups/%s", baseUrl, plane, resourceGroupName),
		strings.NewReader(`{}`),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create UCP resourceGroup: %w", err)
	}
	resp, err := rt.RoundTrip(createRgRequest)
	if err != nil {
		return "", fmt.Errorf("failed to create UCP resourceGroup: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request to create UCP resouceGroup failed with status: %d, request: %+v", resp.StatusCode, resp)
	}
	defer resp.Body.Close()
	var jsonBody map[string]interface{}
	if json.NewDecoder(resp.Body).Decode(&jsonBody) != nil {
		return "", nil
	}

	return jsonBody["id"].(string), nil
}

func createEnvironmentResource(ctx context.Context, kubeCtxName, resourceGroupName, environmentName string) (string, error) {
	_, conn, err := kubernetes.CreateAPIServerConnection(kubeCtxName, "")
	if err != nil {
		return "", err
	}

	loc := "global"
	id := "self"

	toCreate := coreRpApps.EnvironmentResource{
		TrackedResource: coreRpApps.TrackedResource{
			Location: &loc,
		},
		Properties: &coreRpApps.EnvironmentProperties{
			Compute: &coreRpApps.KubernetesCompute{
				EnvironmentCompute: coreRpApps.EnvironmentCompute{
					Kind:       to.StringPtr(coreRpApps.EnvironmentComputeKindKubernetes),
					ResourceID: &id,
				},
				// FIXME: allow users to specify kubernetes namespace
				Namespace: to.StringPtr(environmentName),
			},
		},
	}

	rootScope := fmt.Sprintf("planes/radius/local/resourceGroups/%s", resourceGroupName)
	c := coreRpApps.NewEnvironmentsClient(conn, rootScope)
	resp, err := c.CreateOrUpdate(ctx, environmentName, toCreate, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create Applications.Core/environments resource: %w", err)
	}
	return *resp.ID, nil
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

func selectNamespace(cmd *cobra.Command, defaultVal string, interactive bool) (string, error) {
	var val string
	var err error
	if interactive {
		promptMsg := fmt.Sprintf("Enter a namespace name to deploy apps into [%s]:", defaultVal)
		val, err = prompt.TextWithDefault(promptMsg, &defaultVal, prompt.EmptyValidator)
		if err != nil {
			return "", err
		}
		fmt.Printf("Using %s as namespace name\n", val)
	} else {
		val, _ = cmd.Flags().GetString("namespace")
		if val == "" {
			output.LogInfo("No namespace name provided, using: %v", defaultVal)
			val = defaultVal
		}
	}
	return val, nil
}

func selectEnvironment(cmd *cobra.Command, defaultVal string, interactive bool) (string, error) {
	var val string
	var err error
	if interactive {
		promptMsg := fmt.Sprintf("Enter an environment name [%s]:", defaultVal)
		val, err = prompt.TextWithDefault(promptMsg, &defaultVal, prompt.EmptyValidator)
		if err != nil {
			return "", err
		}
		fmt.Printf("Using %s as environment name\n", val)
	} else {
		val, _ = cmd.Flags().GetString("environment")
		if val == "" {
			output.LogInfo("No environment name provided, using: %v", defaultVal)
			val = defaultVal
		}
	}
	return val, nil
}
