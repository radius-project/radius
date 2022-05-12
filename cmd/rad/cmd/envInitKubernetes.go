// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

var envInitKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Initializes a kubernetes environment",
	Long:  `Initializes a kubernetes environment.`,
	RunE:  installKubernetes,
}

func installKubernetes(cmd *cobra.Command, args []string) error {
	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	chartPath, err := cmd.Flags().GetString("chart")
	if err != nil {
		return err
	}

	image, err := cmd.Flags().GetString("image")
	if err != nil {
		return err
	}

	tag, err := cmd.Flags().GetString("tag")
	if err != nil {
		return err
	}

	client, runtimeClient, contextName, err := createKubernetesClients("")
	if err != nil {
		return err
	}

	// Configure Azure provider for cloud resources if specified
	var azureProvider *azure.Provider
	if interactive {
		var defaultNamespace = "default"
		promptStr := fmt.Sprintf("Enter a namespace name to deploy apps into [%s]:", defaultNamespace)
		namespace, err = prompt.TextWithDefault(promptStr, &defaultNamespace, prompt.EmptyValidator)
		if err != nil {
			return err
		}
		fmt.Printf("Using %s as namespace name\n", namespace)

		var defaultEnvironmentName = contextName
		promptStr = fmt.Sprintf("Enter an environment name [%s]:", defaultEnvironmentName)
		environmentName, err = prompt.TextWithDefault(promptStr, &defaultEnvironmentName, prompt.EmptyValidator)
		if err != nil {
			return err
		}
		fmt.Printf("Using %s as environment name\n", environmentName)

	} else {
		// Check if Azure provider configuration is provided
		// Adding Azure provider is supported only in non-interactive mode
		addAzure, err := cmd.Flags().GetBool("provider-azure")
		if err != nil {
			return err
		}
		if addAzure {
			azureProvider, err = readAzureProviderNonInteractive(cmd)
			if err != nil {
				return err
			}
			if azureProvider == nil {
				return errors.New("Failed to configure Azure provider")
			}
		}
	}

	config := ConfigFromContext(cmd.Context())
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	if environmentName == "" {
		environmentName = contextName
		output.LogInfo("No environment name provided, using: %v", environmentName)
	}

	_, foundConflict := env.Items[environmentName]
	if foundConflict {
		return fmt.Errorf("an environment named %s already exists. Use `rad env delete %s` to delete or select a different name", environmentName, environmentName)
	}

	step := output.BeginStep("Installing Radius...")

	cliOptions := helm.ClusterOptions{
		Namespace: namespace,
		Radius: helm.RadiusOptions{
			ChartPath: chartPath,
			Image:     image,
			Tag:       tag,
		},
	}
	clusterOptions := helm.NewClusterOptions(cliOptions)

	clusterOptions.Radius.AzureProvider = azureProvider

	err = helm.InstallOnCluster(cmd.Context(), clusterOptions, client, runtimeClient)
	if err != nil {
		return err
	}

	output.CompleteStep(step)

	env.Items[environmentName] = map[string]interface{}{
		"kind":      environments.KindKubernetes,
		"context":   contextName,
		"namespace": namespace,
	}

	err = cli.SaveConfigOnLock(cmd.Context(), config, cli.UpdateEnvironmentWithLatestConfig(env, cli.MergeInitEnvConfig(environmentName)))
	if err != nil {
		return err
	}

	return nil
}

func readAzureProviderNonInteractive(cmd *cobra.Command) (*azure.Provider, error) {
	subscriptionID, err := cmd.Flags().GetString("provider-azure-subscription")
	if err != nil {
		return nil, err
	}

	if subscriptionID == "" {
		return nil, fmt.Errorf("--provider-azure-subscription is required to configure Azure provider for cloud resources")
	}

	resourceGroup, err := cmd.Flags().GetString("provider-azure-resource-group")
	if err != nil {
		return nil, err
	}

	if resourceGroup == "" {
		return nil, fmt.Errorf("--provider-azure-resource-group is required to configure Azure provider for cloud resources")
	}

	// Verify that all required properties for the service principal have been specified
	clientID, err := cmd.Flags().GetString("provider-azure-client-id")
	if err != nil {
		return nil, err
	}

	if clientID == "" {
		return nil, errors.New("--provider-azure-client-id parameter is required to configure Azure provider for cloud resources")
	}

	clientSecret, err := cmd.Flags().GetString("provider-azure-client-secret")
	if err != nil {
		return nil, err
	}
	if clientSecret == "" {
		return nil, errors.New("--provider-azure-client-secret parameter is required to configure Azure provider for cloud resources")
	}

	tenantID, err := cmd.Flags().GetString("provider-azure-tenant-id")
	if err != nil {
		return nil, err
	}

	if tenantID == "" {
		return nil, errors.New("--provider-azure-tenant-id parameter is required to configure Azure provider for cloud resources")
	}

	return &azure.Provider{
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
		ServicePrincipal: &azure.ServicePrincipal{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TenantID:     tenantID,
		},
	}, nil
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
	envInitKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInitKubernetesCmd.Flags().StringP("namespace", "n", "default", "The namespace to use for the environment")
	envInitKubernetesCmd.Flags().StringP("chart", "", "", "Specify a file path to a helm chart to install radius from")
	envInitKubernetesCmd.Flags().String("image", "", "Specify the radius controller image to use")
	envInitKubernetesCmd.Flags().String("tag", "", "Specify the radius controller tag to use")

	// Parameters to configure Azure provider for cloud resources
	envInitKubernetesCmd.Flags().BoolP("provider-azure", "", false, "Add Azure provider for cloud resources")
	envInitKubernetesCmd.Flags().String("provider-azure-subscription", "", "Azure subscription for cloud resources")
	envInitKubernetesCmd.Flags().String("provider-azure-resource-group", "", "Azure resource-group for cloud resources")
	envInitKubernetesCmd.Flags().StringP("provider-azure-client-id", "", "", "The client id for the service principal")
	envInitKubernetesCmd.Flags().StringP("provider-azure-client-secret", "", "", "The client secret for the service principal")
	envInitKubernetesCmd.Flags().StringP("provider-azure-tenant-id", "", "", "The tenant id for the service principal")
}
