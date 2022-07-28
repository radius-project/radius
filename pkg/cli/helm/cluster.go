// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/project-radius/radius/pkg/version"
)

const (
	ContourChartDefaultVersion = "7.4.6"
	DaprDefaultVersion         = "1.6.0"
)

type CLIClusterOptions struct {
	Radius RadiusOptions
}

type ClusterOptions struct {
	Dapr    DaprOptions
	Contour ContourOptions
	Radius  RadiusOptions
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
		Dapr: DaprOptions{
			Version: DaprDefaultVersion,
		},
		Contour: ContourOptions{
			ChartVersion: ContourChartDefaultVersion,
		},
		Radius: RadiusOptions{
			ChartVersion: chartVersion,
			Tag:          tag,
			AppCoreTag:   tag,
			UCPTag:       tag,
			DETag:        tag,
		},
	}
}

func PopulateDefaultClusterOptions(cliOptions CLIClusterOptions) ClusterOptions {
	options := NewDefaultClusterOptions()

	// If any of the CLI options are provided, override the default options.
	if cliOptions.Radius.Reinstall {
		options.Radius.Reinstall = cliOptions.Radius.Reinstall
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

	if cliOptions.Radius.AppCoreImage != "" {
		options.Radius.AppCoreImage = cliOptions.Radius.AppCoreImage
	}

	if cliOptions.Radius.AppCoreTag != "" {
		options.Radius.AppCoreTag = cliOptions.Radius.AppCoreTag
	}

	if cliOptions.Radius.UCPImage != "" {
		options.Radius.UCPImage = cliOptions.Radius.UCPImage
	}

	if cliOptions.Radius.UCPTag != "" {
		options.Radius.UCPTag = cliOptions.Radius.UCPTag
	}

	if cliOptions.Radius.PublicEndpointOverride != "" {
		options.Radius.PublicEndpointOverride = cliOptions.Radius.PublicEndpointOverride
	}

	if cliOptions.Radius.AzureProvider != nil {
		options.Radius.AzureProvider = cliOptions.Radius.AzureProvider
	}

	return options
}

func InstallOnCluster(ctx context.Context, options ClusterOptions, kubeContext string) (bool, error) {
	// Do note: the namespace passed in to rad install kubernetes
	// doesn't match the namespace where we deploy radius.
	// The RPs and other resources are all deployed to the
	// 'radius-system' namespace. The namespace passed in will be
	// where pods/services/deployments will be put for rad deploy.
	foundExisting, err := ApplyRadiusHelmChart(options.Radius, kubeContext)
	if err != nil {
		return false, err
	}

	err = ApplyContourHelmChart(options.Contour, kubeContext)
	if err != nil {
		return false, err
	}

	err = ApplyDaprHelmChart(options.Dapr.Version, kubeContext)
	if err != nil {
		return false, err
	}

	return foundExisting, err
}

func UninstallOnCluster(kubeContext string) error {
	var helmOutput strings.Builder

	namespace := RadiusSystemNamespace
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}

	helmConf, err := HelmConfig(&helmOutput, &flags)
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
