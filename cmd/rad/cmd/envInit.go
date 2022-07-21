// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/k3d"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	coreRpApps "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
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
	envInitCmd.PersistentFlags().BoolP("force", "f", false, "Overwrite existing workspace if present")

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

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	force, err := cmd.Flags().GetBool("force")
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

	environmentName, err := selectEnvironmentName(cmd, defaultEnvName, interactive)
	if err != nil {
		return err
	}

	params := &EnvironmentParams{
		Name:      environmentName,
		Providers: &environments.Providers{AzureProvider: azureProvider},
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

	var k8sGoClient client_go.Interface
	var contextName string
	var registry *workspaces.Registry
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
		contextName = cluster.ContextName
		registry = &workspaces.Registry{
			PushEndpoint: cluster.RegistryPushEndpoint,
			PullEndpoint: cluster.RegistryPullEndpoint,
		}

	case Kubernetes:
		k8sGoClient, _, contextName, err = createKubernetesClients("")
		if err != nil {
			return err
		}
	}

	// Fallback logic for workspace
	//
	// - passed via flag
	// - default value (config)
	// - environment name
	workspaceName, err := cmd.Flags().GetString("workspace")
	if err != nil {
		return err
	}

	if workspaceName == "" {
		section, err := cli.ReadWorkspaceSection(config)
		if err != nil {
			return err
		}

		workspaceName = section.Default
	}

	if workspaceName == "" {
		workspaceName = environmentName
	}

	// We're going to update the workspace in place if it's compatible. We only need to
	// report an error if it's not (eg: different connection type or different kubecontext.)
	foundExistingWorkspace, err := cli.HasWorkspace(config, workspaceName)
	if err != nil {
		return err
	}

	if (!isEmpty(chartArgs) || azureProvider != nil) && !chartArgs.Reinstall && foundExistingWorkspace {
		return fmt.Errorf("chart arg / provider config is not empty for existing workspace. Specify '--reinstall' for the new arguments to take effect")
	}

	var workspace *workspaces.Workspace
	if foundExistingWorkspace {
		workspace, err = cli.GetWorkspace(config, workspaceName)
		if err != nil {
			return err
		}
	}

	if foundExistingWorkspace && !force && !workspace.ConnectionEquals(&workspaces.KubernetesConnection{Kind: workspaces.KindKubernetes, Context: contextName}) {
		return fmt.Errorf("the workspace %q already exists. Specify '--force' to overwrite", workspaceName)
	}

	// Make sure namespace for applications exists
	err = kubernetes.EnsureNamespace(cmd.Context(), k8sGoClient, namespace)
	if err != nil {
		return err
	}

	foundExistingRadius, err := setup.Install(cmd.Context(), clusterOptions, contextName)
	if err != nil {
		return err
	}

	//If existing radius control plane, retrieve az provider subscription and resourcegroup, and use that unless a --reinstall is specified
	var azProviderFromInstall *azure.Provider
	if foundExistingRadius {
		azProviderFromInstall, err = helm.GetAzProvider(cliOptions.Radius, contextName)
		if err != nil {
			return err
		}
		azProviderFromInstall.ServicePrincipal.ClientID = "*****"
		azProviderFromInstall.ServicePrincipal.ClientSecret = "*****"
		azProviderFromInstall.ServicePrincipal.TenantID = "*****"

	}

	// we dont have a workspace, but looks like a az provider has been previously configured through rad install kubernetes. Are we trying to key in new azprovider?
	//log info suggesting we need a --reinstall if we want to use the new spn
	if (azureProvider != nil) && !chartArgs.Reinstall && (azProviderFromInstall != nil) {
		output.LogInfo("provider config is not empty but radius already has an one configured. Rerun rad env init with '--reinstall' for the new provider config to take effect, otherwise radius will use the existing config")
	}

	// Steps:
	//
	// 1. Create workspace & resource groups
	// 2. Create environment resource
	// 3. Update workspace
	step := output.BeginStep("Creating Workspace...")

	// TODO: we TEMPORARILY create a resource group as part of creating the workspace.
	//
	// We'll flesh this out more when we add explicit commands for managing resource groups.
	id, err := setup.CreateWorkspaceResourceGroup(cmd.Context(), &workspaces.KubernetesConnection{Context: contextName}, workspaceName)
	if err != nil {
		return err
	}

	workspace = &workspaces.Workspace{
		Connection: map[string]interface{}{
			"kind":    "kubernetes",
			"context": contextName,
		},
		Scope:    id,
		Registry: registry,
	}

	if azureProvider != nil {
		workspace.ProviderConfig.Azure = &workspaces.Provider{
			SubscriptionID: azureProvider.SubscriptionID,
			ResourceGroup:  azureProvider.ResourceGroup,
		}
	} else if azProviderFromInstall != nil {
		workspace.ProviderConfig.Azure = &workspaces.Provider{
			SubscriptionID: azProviderFromInstall.SubscriptionID,
			ResourceGroup:  azProviderFromInstall.ResourceGroup,
		}
	}

	err = cli.EditWorkspaces(cmd.Context(), config, func(section *cli.WorkspaceSection) error {
		section.Default = workspaceName
		section.Items[strings.ToLower(workspaceName)] = *workspace
		return nil
	})
	if err != nil {
		return err
	}

	output.LogInfo("Set %q as current workspace", workspaceName)
	output.CompleteStep(step)

	// Reload config so we can see the updates
	config, err = cli.LoadConfig(config.ConfigFileUsed())
	if err != nil {
		return err
	}

	step = output.BeginStep("Creating Environment...")

	scopeId, err := resources.Parse(workspace.Scope)
	if err != nil {
		return err
	}

	environmentID, err := createEnvironmentResource(cmd.Context(), contextName, scopeId.FindScope(resources.ResourceGroupsSegment), environmentName, namespace)
	if err != nil {
		return err
	}

	err = cli.EditWorkspaces(cmd.Context(), config, func(section *cli.WorkspaceSection) error {
		ws := section.Items[strings.ToLower(workspaceName)]
		ws.Environment = environmentID
		section.Items[strings.ToLower(workspaceName)] = ws
		return nil
	})
	if err != nil {
		return err
	}

	output.LogInfo("Set %q as current environment for workspace %q", environmentName, workspaceName)
	output.CompleteStep(step)

	return nil
}

func createEnvironmentResource(ctx context.Context, kubeCtxName, resourceGroupName, environmentName string, namespace string) (string, error) {
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
				Namespace: to.StringPtr(namespace),
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

func selectEnvironmentName(cmd *cobra.Command, defaultVal string, interactive bool) (string, error) {
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

func isEmpty(chartArgs *setup.ChartArgs) bool {
	var emptyChartArgs setup.ChartArgs
	return (chartArgs == nil || *chartArgs == emptyChartArgs)
}
