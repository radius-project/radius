// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	k8slabels "github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/kubernetes/kubectl"
	"github.com/project-radius/radius/pkg/version"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
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

	step := output.BeginStep("Installing Radius...")

	client, runtimeClient, contextName, err := createKubernetesClients("")
	if err != nil {
		return err
	}

	err = installRadius(cmd.Context(), client, runtimeClient, namespace, chartPath, image, tag)
	if err != nil {
		return err
	}

	err = installGateway(cmd.Context(), runtimeClient, helm.HAProxyOptions{UseHostNetwork: true})
	if err != nil {
		return err
	}

	err = installDapr(cmd.Context(), runtimeClient)
	if err != nil {
		return err
	}

	output.CompleteStep(step)

	config := ConfigFromContext(cmd.Context())

	env, err := cli.ReadEnvironmentSection(config)
	if err != nil {
		return err
	}

	if environmentName == "" {
		environmentName = contextName
	}

	env.Items[environmentName] = map[string]interface{}{
		"kind":      environments.KindKubernetes,
		"context":   contextName,
		"namespace": namespace,
	}

	cli.UpdateEnvironmentSectionOnCreation(config, env, environmentName)

	err = cli.SaveConfig(config)
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

func installRadius(ctx context.Context, client client_go.Interface, runtimeClient runtime_client.Client, namespace string, chartPath string, image string, tag string) error {
	// Make sure namespace passed in exists.
	err := kubernetes.EnsureNamespace(ctx, client, namespace)
	if err != nil {
		return err
	}

	// Do note: the namespace passed in to rad env init kubernetes
	// doesn't match the namespace where we deploy the controller to.
	// The controller and other resources are all deployed to the
	// 'radius-system' namespace. The namespace passed in will be
	// where pods/services/deployments will be put for rad deploy.
	err = kubernetes.EnsureNamespace(ctx, client, helm.RadiusSystemNamespace)
	if err != nil {
		return err
	}

	// chartVersion is optional for an edge build - we'll get the latest available
	// in that case
	chartVersion := ""
	if !version.IsEdgeChannel() {
		// For a non-edge CLI we want to install the latest build that matches
		// the current channel.
		chartVersion = fmt.Sprintf("~%s.0", version.Channel())
	}

	err = helm.ApplyRadiusHelmChart(chartPath, chartVersion, image, tag)
	if err != nil {
		return err
	}

	return nil
}

func installGateway(ctx context.Context, runtimeClient runtime_client.Client, options helm.HAProxyOptions) error {
	err := kubectl.RunCLICommandSilent("apply", "--kustomize", fmt.Sprintf("github.com/kubernetes-sigs/gateway-api/config/crd?ref=%s", GatewayCRDVersion))
	if err != nil {
		return err
	}

	err = helm.ApplyHAProxyHelmChart(HAProxyVersion, options)
	if err != nil {
		return err
	}

	err = applyGatewayClass(ctx, runtimeClient)
	if err != nil {
		return err
	}

	return nil
}

func applyGatewayClass(ctx context.Context, runtimeClient runtime_client.Client) error {
	gateway := gatewayv1alpha1.GatewayClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GatewayClass",
			APIVersion: "networking.x-k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "haproxy",
			Namespace: "radius-system",
		},
		Spec: gatewayv1alpha1.GatewayClassSpec{
			Controller: "haproxy-ingress.github.io/controller",
		},
	}

	err := runtimeClient.Patch(ctx, &gateway, runtime_client.Apply, &runtime_client.PatchOptions{FieldManager: k8slabels.FieldManager})
	return err
}

func installDapr(ctx context.Context, runtimeClient runtime_client.Client) error {
	err := helm.ApplyDaprHelmChart(DaprVersion)
	if err != nil {
		return err
	}

	return nil
}

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
	envInitKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInitKubernetesCmd.Flags().StringP("namespace", "n", "default", "The namespace to use for the environment")
	envInitKubernetesCmd.Flags().StringP("chart", "", "", "Specify a file path to a helm chart to install radius from")
	envInitKubernetesCmd.Flags().String("image", "", "Specify the radius controller image to use")
	envInitKubernetesCmd.Flags().String("tag", "", "Specify the radius controller tag to use")
}
