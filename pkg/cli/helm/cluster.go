// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
	k8slabels "github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/kubernetes/kubectl"
	"github.com/project-radius/radius/pkg/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	HAProxyDefaultVersion    = "0.13.4"
	GatewayCRDDefaultVersion = "v0.3.0"
	DaprDefaultVersion       = "1.6.0"
)

type ClusterOptions struct {
	Namespace string
	Dapr      DaprOptions
	HAProxy   HAProxyOptions
	Radius    RadiusOptions
}

func NewDefaultClusterOptions() ClusterOptions {
	// By default we use the chart version that matches the channel of the CLI (major.minor)
	// If this is an edge build, we'll use the latest available.
	chartVersion := version.ChartVersion()
	if !version.IsEdgeChannel() {
		chartVersion = fmt.Sprintf("~%s", version.ChartVersion())
	}

	return ClusterOptions{
		Namespace: "default",
		Dapr: DaprOptions{
			Version: DaprDefaultVersion,
		},
		HAProxy: HAProxyOptions{
			ChartVersion:      HAProxyDefaultVersion,
			GatewayCRDVersion: GatewayCRDDefaultVersion,
			UseHostNetwork:    true,
		},
		Radius: RadiusOptions{
			ChartVersion: chartVersion,
		},
	}
}

func InstallOnCluster(ctx context.Context, options ClusterOptions, client client_go.Interface, runtimeClient runtime_client.Client) error {
	// Make sure namespace passed in exists.
	err := kubernetes.EnsureNamespace(ctx, client, options.Namespace)
	if err != nil {
		return err
	}

	// Do note: the namespace passed in to rad env init kubernetes
	// doesn't match the namespace where we deploy the controller to.
	// The controller and other resources are all deployed to the
	// 'radius-system' namespace. The namespace passed in will be
	// where pods/services/deployments will be put for rad deploy.
	err = kubernetes.EnsureNamespace(ctx, client, RadiusSystemNamespace)
	if err != nil {
		return err
	}

	err = ApplyRadiusHelmChart(options.Radius)
	if err != nil {
		return err
	}

	err = installGateway(ctx, runtimeClient, options.HAProxy)
	if err != nil {
		return err
	}

	err = ApplyDaprHelmChart(options.Dapr.Version)
	if err != nil {
		return err
	}

	return err
}

func UninstallOnCluster(ctx context.Context) error {
	err := kubectl.RunCLICommandSilent("delete", "gatewayclasses", "haproxy", "--ignore-not-found", "true")
	if err != nil {
		return err
	}

	var helmOutput strings.Builder
	helmConf, err := HelmConfig(RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	err = RunDaprHelmUninstall(helmConf)
	if err != nil {
		return err
	}

	err = RunHAProxyHelmUninstall(helmConf)
	if err != nil {
		return err
	}

	err = RunRadiusHelmUninstall(helmConf)
	if err != nil {
		return err
	}

	return nil
}

func installGateway(ctx context.Context, runtimeClient runtime_client.Client, options HAProxyOptions) error {
	err := kubectl.RunCLICommandSilent("apply", "--kustomize", fmt.Sprintf("github.com/kubernetes-sigs/gateway-api/config/crd?ref=%s", options.GatewayCRDVersion))
	if err != nil {
		return err
	}

	err = ApplyHAProxyHelmChart(options)
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
