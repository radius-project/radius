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

	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/Azure/radius/pkg/cli/helm"
	"github.com/Azure/radius/pkg/cli/kubernetes"
	"github.com/Azure/radius/pkg/cli/output"
	"github.com/Azure/radius/pkg/cli/prompt"
	k8slabels "github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/kubernetes/kubectl"
	"github.com/Azure/radius/pkg/version"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	HAProxyVersion    = "0.13.4"
	GatewayCRDVersion = "v0.3.0"
)

var envInitKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Initializes a kubernetes environment",
	Long:  `Initializes a kubernetes environment`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		k8sconfig, err := kubernetes.ReadKubeConfig()
		if err != nil {
			return err
		}

		if k8sconfig.CurrentContext == "" {
			return errors.New("no kubernetes context is set")
		}

		context := k8sconfig.Contexts[k8sconfig.CurrentContext]
		if context == nil {
			return fmt.Errorf("kubernetes context '%s' could not be found", k8sconfig.CurrentContext)
		}

		step := output.BeginStep("Installing Radius...")

		client, _, err := kubernetes.CreateTypedClient(k8sconfig.CurrentContext)
		if err != nil {
			return err
		}

		// Do note: the namespace passed in to rad env init kubernetes
		// doesn't match the namespace where we deploy the controller to.
		// The controller and other resources are all deployed to the
		// 'radius-system' namespace. The namespace passed in will be
		// where pods/services/deployments will be put for rad deploy.
		err = kubernetes.CreateNamespace(cmd.Context(), client, helm.RadiusSystemNamespace)
		if err != nil {
			return err
		}

		err = kubectl.RunCLICommandSilent("apply", "--kustomize", fmt.Sprintf("github.com/kubernetes-sigs/gateway-api/config/crd?ref=%s", GatewayCRDVersion))
		if err != nil {
			return err
		}

		err = helm.ApplyHAProxyHelmChart(HAProxyVersion)
		if err != nil {
			return err
		}

		runtimeClient, err := kubernetes.CreateRuntimeClient(k8sconfig.CurrentContext, kubernetes.Scheme)
		if err != nil {
			return err
		}

		err = applyGatewayClass(cmd.Context(), runtimeClient)
		if err != nil {
			return err
		}

		err = helm.ApplyRadiusHelmChart(chartPath, version.NewVersionInfo().Channel, image, tag)
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
			environmentName = k8sconfig.CurrentContext
		}

		env.Items[environmentName] = map[string]interface{}{
			"kind":      environments.KindKubernetes,
			"context":   k8sconfig.CurrentContext,
			"namespace": namespace,
		}

		output.LogInfo("Using environment: %v", environmentName)
		env.Default = environmentName
		cli.UpdateEnvironmentSection(config, env)

		err = cli.SaveConfig(config)
		if err != nil {
			return err
		}

		return nil
	},
}

func applyGatewayClass(ctx context.Context, runtimeClient sigclient.Client) error {
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

	err := runtimeClient.Patch(ctx, &gateway, sigclient.Apply, &client.PatchOptions{FieldManager: k8slabels.FieldManager})
	return err
}

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
	envInitKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInitKubernetesCmd.Flags().StringP("namespace", "n", "default", "The namespace to use for the environment")
	envInitKubernetesCmd.Flags().StringP("chart", "c", "", "Specify a file path to a helm chart to install radius from")
	envInitKubernetesCmd.Flags().String("image", "", "Specify the radius controller image to use")
	envInitKubernetesCmd.Flags().String("tag", "", "Specify the radius controller tag to use")
}
