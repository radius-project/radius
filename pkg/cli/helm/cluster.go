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
	"github.com/project-radius/radius/pkg/version"
	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ContourDefaultVersion = ""
	DaprDefaultVersion    = "1.6.0"
)

type ClusterOptions struct {
	Namespace string
	Dapr      DaprOptions
	Contour   ContourOptions
	Radius    RadiusOptions
}

func NewDefaultClusterOptions() ClusterOptions {
	// By default we use the chart version that matches the channel of the CLI (major.minor)
	// If this is an edge build, we'll use the latest available.
	chartVersion := version.ChartVersion()
	if !version.IsEdgeChannel() {
		chartVersion = fmt.Sprintf("~%s", version.ChartVersion())
	}

	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}

	return ClusterOptions{
		Namespace: "default",
		Dapr: DaprOptions{
			Version: DaprDefaultVersion,
		},
		Contour: ContourOptions{
			ChartVersion: ContourDefaultVersion,
		},
		Radius: RadiusOptions{
			ChartVersion: chartVersion,
			Tag:          tag,
		},
	}
}

func NewClusterOptions(cliOptions ClusterOptions) ClusterOptions {
	options := NewDefaultClusterOptions()

	// If any of the CLI options are provided, override the default options.

	if cliOptions.Namespace != "" {
		options.Namespace = cliOptions.Namespace
	}

	if cliOptions.Radius.ChartPath != "" {
		options.Radius.ChartPath = cliOptions.Radius.ChartPath
	}

	if cliOptions.Radius.Image != "" {
		options.Radius.Image = cliOptions.Radius.Image
	}

	if cliOptions.Radius.Tag != "" {
		options.Radius.Tag = cliOptions.Radius.Tag
	}

	return options
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

	err = ApplyContourHelmChart(options.Contour)
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
	var helmOutput strings.Builder
	helmConf, err := HelmConfig(RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	err = RunDaprHelmUninstall(helmConf)
	if err != nil {
		return err
	}

	err = RunContourHelmUninstall(helmConf)
	if err != nil {
		return err
	}

	err = RunRadiusHelmUninstall(helmConf)
	if err != nil {
		return err
	}

	return nil
}
