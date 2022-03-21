// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/mitchellh/mapstructure"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/k3d"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
)

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

	// Gather inputs and validate
	interactive, err := cmd.Flags().GetBool("interactive")
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

	var params *DevEnvironmentParams
	if interactive {
		params, err = envInitDevConfigInteractive(cmd)
		if err != nil {
			return err
		}
	} else {
		params, err = envInitDevConfigNonInteractive(cmd)
		if err != nil {
			return err
		}
	}

	_, foundConflict := env.Items[params.Name]
	if foundConflict {
		return fmt.Errorf("an environment named %s already exists. Use `rad env delete %s to delete or select a different name", params.Name, params.Name)
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

	namespace := "default"
	err = installRadius(cmd.Context(), client, runtimeClient, namespace, chartPath, image, tag)
	if err != nil {
		return err
	}

	// We don't want to use the host network option with HA Proxy on K3d. K3d supports LoadBalancer services,
	// using the host network would cause a conflict.
	err = installGateway(cmd.Context(), runtimeClient, helm.HAProxyOptions{UseHostNetwork: false})
	if err != nil {
		return err
	}

	output.CompleteStep(step)

	// Persist settings
	env.Items[params.Name] = map[string]interface{}{
		"kind":        "dev",
		"context":     cluster.ContextName,
		"clustername": cluster.ClusterName,
		"namespace":   "default",
		"registry": &environments.Registry{
			PushEndpoint: cluster.RegistryPushEndpoint,
			PullEndpoint: cluster.RegistryPullEndpoint,
		},
	}

	if params.Providers != nil {
		providerData := map[string]interface{}{}
		err = mapstructure.Decode(params.Providers, &providerData)
		if err != nil {
			return err
		}

		env.Items[params.Name]["providers"] = providerData
	}

	err = SaveConfig(cmd.Context(), config, UpdateEnvironmentSectionOnCreation(params.Name, env, cli.Init))
	if err != nil {
		return err
	}

	return nil
}

func envInitDevConfigInteractive(cmd *cobra.Command) (*DevEnvironmentParams, error) {
	name, err := prompt.Text("Enter an environment name:", prompt.EmptyValidator)
	if err != nil {
		return nil, err
	}

	if name == "" {
		name = "dev"
	}

	params := DevEnvironmentParams{
		Name: name,
	}

	addAzure, err := prompt.Confirm("Add Azure provider for cloud resources [y/n]?")
	if err != nil {
		return nil, err
	} else if !addAzure {
		return &params, nil
	}

	authorizer, err := auth.NewAuthorizerFromCLI()
	if err != nil {
		return nil, err
	}

	subscription, err := selectSubscription(cmd.Context(), authorizer)
	if err != nil {
		return nil, err
	}

	resourceGroup, err := selectResourceGroup(cmd.Context(), authorizer, subscription)
	if err != nil {
		return nil, err
	}

	params.Providers = &environments.Providers{
		AzureProvider: &environments.AzureProvider{
			SubscriptionID: subscription.SubscriptionID,
			ResourceGroup:  resourceGroup,
		},
	}

	return &params, nil
}

func envInitDevConfigNonInteractive(cmd *cobra.Command) (*DevEnvironmentParams, error) {
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return nil, err
	}

	subscriptionID, err := cmd.Flags().GetString("provider-azure-subscription")
	if err != nil {
		return nil, err
	}

	resourceGroup, err := cmd.Flags().GetString("provider-azure-resource-group")
	if err != nil {
		return nil, err
	}

	params := DevEnvironmentParams{
		Name: name,
	}

	if (subscriptionID != "") != (resourceGroup != "") {
		return nil, fmt.Errorf("to use the Azure provider both --provider-azure-subscription and --provider-azure-resource-group must be provided")
	}

	return &params, nil
}

func init() {
	envInitCmd.AddCommand(envInitLocalCmd)

	envInitLocalCmd.Flags().StringP("name", "n", "dev", "environment name")
	envInitLocalCmd.Flags().BoolP("interactive", "i", false, "interactively prompt for environment information")

	// TODO: right now we only handle Azure as a special case. This needs to be generalized
	// to handle other providers.
	envInitLocalCmd.Flags().String("provider-azure-subscription", "", "Azure subscription for cloud resources")
	envInitLocalCmd.Flags().String("provider-azure-resource-group", "", "Azure resource-group for cloud resources")

	envInitLocalCmd.Flags().String("chart", "", "specify a file path to a helm chart to install Radius from")
	envInitLocalCmd.Flags().String("image", "", "specify the radius controller image to use")
	envInitLocalCmd.Flags().String("tag", "", "specify the radius controller tag to use")
}

type DevEnvironmentParams struct {
	Name      string
	Providers *environments.Providers
}
