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
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	HAProxyVersion    = "0.13.4"
	GatewayCRDVersion = "v0.3.0"
	DaprVersion       = "1.6.0"
)

var envInitKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Initializes a kubernetes environment",
	Long:  `Initializes a kubernetes environment`,
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

	if interactive {
		namespace, err = prompt.Text("Enter a namespace name:", prompt.EmptyValidator)
		if err != nil {
			return err
		}
	}

	config := ConfigFromContext(cmd.Context())
	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	client, runtimeClient, contextName, err := createKubernetesClients("")
	if err != nil {
		return err
	}

	if environmentName == "" {
		environmentName = contextName
	}

	_, foundConflict := env.Items[environmentName]
	if foundConflict {
		return fmt.Errorf("an environment named %s already exists. Use `rad env delete %s` to delete or select a different name", environmentName, environmentName)
	}

	step := output.BeginStep("Installing Radius...")

	options := helm.NewDefaultClusterOptions()
	options.Namespace = namespace
	options.Radius.ChartPath = chartPath
	options.Radius.Image = image
	options.Radius.Tag = tag

	err = helm.InstallOnCluster(cmd.Context(), options, client, runtimeClient)
	if err != nil {
		return err
	}

	output.CompleteStep(step)

	env.Items[environmentName] = map[string]interface{}{
		"kind":      environments.KindKubernetes,
		"context":   contextName,
		"namespace": namespace,
	}

	err = SaveConfig(cmd.Context(), config, UpdateEnvironmentSectionOnCreation(environmentName, env, cli.Init))
	if err != nil {
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
	envInitKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInitKubernetesCmd.Flags().StringP("namespace", "n", "default", "The namespace to use for the environment")
	envInitKubernetesCmd.Flags().StringP("chart", "", "", "Specify a file path to a helm chart to install radius from")
	envInitKubernetesCmd.Flags().String("image", "", "Specify the radius controller image to use")
	envInitKubernetesCmd.Flags().String("tag", "", "Specify the radius controller tag to use")
}
