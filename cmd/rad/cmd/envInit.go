// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	coreRpApps "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var envInitCmd = &cobra.Command{
	Use:    "init",
	Short:  "Create a RAD environment",
	Long:   `Create a RAD environment`,
	Hidden: true,
}

func init() {
	envCmd.AddCommand(envInitCmd)
	envInitCmd.PersistentFlags().BoolP("interactive", "i", false, "Collect values for required command arguments through command line interface prompts")
	envInitCmd.PersistentFlags().StringP("namespace", "n", "default", "Specify the namespace to use for the environment into which application resources are deployed")
	envInitCmd.PersistentFlags().BoolP("force", "f", false, "Overwrite existing workspace if present")

	setup.RegisterPersistentChartArgs(envInitCmd)
	setup.RegisterPersistentAzureProviderArgs(envInitCmd)
	setup.RegisterPersistentAWSProviderArgs(envInitCmd)
}

type EnvKind int

const (
	Kubernetes EnvKind = iota
)

func (k EnvKind) String() string {
	return [...]string{"Kubernetes"}[k]
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

	namespace, err := common.SelectNamespace(cmd, "default", interactive, &prompt.Impl{})
	if err != nil {
		return err
	}

	chartArgs, err := setup.ParseChartArgs(cmd)
	if err != nil {
		return err
	}

	// Configure Azure provider for cloud resources if specified
	azProvider, err := setup.ParseAzureProviderArgs(cmd, interactive, &prompt.Impl{})
	if err != nil {
		return err
	}

	// Configure AWS provider for cloud resources if specified
	awsProvider, err := setup.ParseAWSProviderArgs(cmd, interactive, &prompt.Impl{})
	if err != nil {
		return err
	}

	var defaultEnvName string

	switch kind {
	case Kubernetes:
		k8sConfig, err := kubernetes.ReadKubeConfig()
		if err != nil {
			return err
		}
		defaultEnvName = k8sConfig.CurrentContext

	default:
		return fmt.Errorf("unknown environment type: %s", kind)
	}

	environmentName, err := common.SelectEnvironmentName(cmd, defaultEnvName, interactive, &prompt.Impl{})
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
			AzureProvider:          azProvider,
			AWSProvider:            awsProvider,
		},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	var k8sGoClient client_go.Interface
	var contextName string
	var registry *workspaces.Registry
	switch kind {
	case Kubernetes:
		k8sGoClient, _, contextName, err = kubernetes.CreateKubernetesClients("")
		if err != nil {
			return err
		}
	}

	// Fallback logic for workspace
	//
	// - passed via flag
	// - default value (config)
	// - environment name

	workspaceSpecified := false
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
	} else {
		workspaceSpecified = true
	}

	// if user does not specify a workspace name and there is no default workspace, use environmentName as workspace name
	if workspaceName == "" {
		workspaceName = environmentName
	}

	matched, msg, _ := prompt.ResourceName(workspaceName)
	if !matched {
		return fmt.Errorf("%s %s. Use --workspace option to specify the valid name", workspaceName, msg)
	}

	// We're going to update the workspace in place if it's compatible. We only need to
	// report an error if it's not (eg: different connection type or different kubecontext.)
	foundExistingWorkspace, err := cli.HasWorkspace(config, workspaceName)
	if err != nil {
		return err
	}

	var workspace *workspaces.Workspace
	if foundExistingWorkspace {
		workspace, err = cli.GetWorkspace(config, workspaceName)
		if err != nil {
			return err
		}
	}

	// If the user specifies a workspace name with -w and that workspace points to a different Kubernetes cluster, then --force is required
	// If the user does not specify a workspace with -w and the current default workspace points to a different cluster, then create a new workspace
	// If the user does not specify a workspace with -w and the current default workspace points to the same cluster, then update the existing workspace

	isSameConn := true
	if foundExistingWorkspace {
		isSameConn = workspace.ConnectionConfigEquals(&workspaces.KubernetesConnectionConfig{Kind: workspaces.KindKubernetes, Context: contextName})
	}

	if foundExistingWorkspace {
		if workspaceSpecified && !force && !isSameConn {
			return fmt.Errorf("the specified workspace %q has a connection to Kubernetes context %q, which is different from context %q. Specify '--force' to overwrite", workspaceName, workspace.Connection["context"], contextName)
		}
		if !workspaceSpecified && !isSameConn {
			workspaceName = environmentName
			workspace = nil
		}
	}

	// Make sure namespace for applications exists
	err = kubernetes.EnsureNamespace(cmd.Context(), k8sGoClient, namespace)
	if err != nil {
		return err
	}

	foundExistingRadius, err := helm.Install(cmd.Context(), clusterOptions, contextName)
	if err != nil {
		return err
	}

	if (azProvider != nil) && !chartArgs.Reinstall && foundExistingRadius {
		return fmt.Errorf("error: adding a cloud provider requires a reinstall of the Radius services. Specify '--reinstall' for the new arguments to take effect")
	}

	if !isEmpty(chartArgs) && !chartArgs.Reinstall && foundExistingRadius {
		return fmt.Errorf("error: arguments provided requires a reinstall of the Radius services. Specify '--reinstall' for the new arguments to take effect")
	}

	//If existing radius control plane, retrieve az provider subscription and resourcegroup, and use that unless a --reinstall is specified
	var azProviderConfigFromInstall *azure.Provider
	if foundExistingRadius {
		azProviderConfigFromInstall, err = helm.GetAzProvider(cliOptions.Radius, contextName)
		if err != nil {
			return err
		}
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
	var connection sdk.Connection
	if workspace != nil {
		connection, err = workspace.Connect()
		if err != nil {
			return err
		}
	} else {
		config, err := kubernetes.GetConfig(contextName)
		if err != nil {
			return err
		}

		connection, err = sdk.NewKubernetesConnectionFromConfig(config)
		if err != nil {
			return err
		}
	}

	id, err := setup.CreateWorkspaceResourceGroup(cmd.Context(), connection, workspaceName)
	if err != nil {
		return err
	}

	workspace = &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": contextName,
		},
		Scope:    id,
		Registry: registry,
		Name:     workspaceName,
	}

	//if reinstall is specified then use azprovider if provided, or if not, this is an install with no azProviderConfig yet.
	//if no reinstall, then make sure to preserve the azProviderConfig from existing installation
	var azProviderConfig workspaces.AzureProvider
	if azProvider != nil {
		azProviderConfig = workspaces.AzureProvider{
			SubscriptionID: azProvider.SubscriptionID,
			ResourceGroup:  azProvider.ResourceGroup,
		}

	} else if azProviderConfigFromInstall != nil && !chartArgs.Reinstall {
		azProviderConfig = workspaces.AzureProvider{
			SubscriptionID: azProviderConfigFromInstall.SubscriptionID,
			ResourceGroup:  azProviderConfigFromInstall.ResourceGroup,
		}
	}

	var awsProviderConfig workspaces.AWSProvider
	if awsProvider != nil {
		awsProviderConfig = workspaces.AWSProvider{
			Region:    awsProvider.TargetRegion,
			AccountId: awsProvider.AccountId,
		}
	}

	err = cli.EditWorkspaces(cmd.Context(), config, func(section *cli.WorkspaceSection) error {
		section.Default = workspaceName
		section.Items[strings.ToLower(workspaceName)] = *workspace
		cli.UpdateAzProvider(section, azProviderConfig, contextName)
		cli.UpdateAWSProvider(section, awsProviderConfig, contextName)
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

	scopeId, err := resources.ParseScope(workspace.Scope)
	if err != nil {
		return err
	}

	environmentID, err := createEnvironmentResource(cmd.Context(), contextName, scopeId.FindScope(resources.ResourceGroupsSegment), environmentName, namespace, azProviderConfig.SubscriptionID, azProviderConfig.ResourceGroup)
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

func createEnvironmentResource(ctx context.Context, kubeCtxName, resourceGroupName, environmentName, namespace, subscriptionID, resourceGroup string) (string, error) {
	config, err := kubernetes.GetConfig(kubeCtxName)
	if err != nil {
		return "", err
	}

	connection, err := sdk.NewKubernetesConnectionFromConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create connection: %w", err)
	}

	id := "self"
	location := v1.LocationGlobal

	toCreate := coreRpApps.EnvironmentResource{
		Location: &location,
		Properties: &coreRpApps.EnvironmentProperties{
			Compute: &coreRpApps.KubernetesCompute{
				Kind:       to.Ptr(coreRpApps.EnvironmentComputeKindKubernetes),
				ResourceID: &id,
				Namespace:  to.Ptr(namespace),
			},
		},
	}

	if subscriptionID != "" && resourceGroup != "" {
		toCreate.Properties.Providers = &coreRpApps.Providers{
			Azure: &coreRpApps.ProvidersAzure{
				Scope: to.Ptr("/subscriptions/" + subscriptionID + "/resourceGroups/" + resourceGroup),
			},
		}
	}

	rootScope := fmt.Sprintf("planes/radius/local/resourceGroups/%s", resourceGroupName)

	envClient, err := coreRpApps.NewEnvironmentsClient(rootScope, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(connection))
	if err != nil {
		return "", fmt.Errorf("failed to create environment client: %w", err)
	}

	resp, err := envClient.CreateOrUpdate(ctx, environmentName, toCreate, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create Applications.Core/environments resource: %w", err)
	}

	return *resp.ID, nil
}

func isEmpty(chartArgs *setup.ChartArgs) bool {
	var emptyChartArgs setup.ChartArgs
	return (chartArgs == nil || *chartArgs == emptyChartArgs)
}
