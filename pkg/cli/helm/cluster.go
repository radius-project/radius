// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	helmaction "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/version"
)

const (
	ContourChartDefaultVersion = "7.10.2"
)

type CLIClusterOptions struct {
	Radius RadiusOptions
}

type ClusterOptions struct {
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
	if cliOptions.Radius.AWSProvider != nil {
		options.Radius.AWSProvider = cliOptions.Radius.AWSProvider
	}
	if len(cliOptions.Radius.Values) > 0 {
		options.Radius.Values = cliOptions.Radius.Values
	}
	return options
}

// Installs radius based on kubecontext in "radius-system" namespace
func Install(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error) {
	step := output.BeginStep("Installing Radius version %s control plane...", version.Version())
	foundExisting, err := InstallOnCluster(ctx, clusterOptions, kubeContext)
	if err != nil {
		return false, err
	}

	output.CompleteStep(step)
	return foundExisting, nil
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

// Checks whethere radius installed on the cluster based of kubeContext
func CheckRadiusInstall(kubeContext string) (bool, error) {
	var helmOutput strings.Builder

	namespace := RadiusSystemNamespace
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}

	helmConf, err := HelmConfig(&helmOutput, &flags)
	if err != nil {
		return false, fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}
	histClient := helmaction.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists

	_, err = histClient.Run(radiusReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

//go:generate mockgen -destination=./mock_cluster.go -package=helm -self_package github.com/project-radius/radius/pkg/cli/helm github.com/project-radius/radius/pkg/cli/helm Interface
type Interface interface {
	CheckRadiusInstall(kubeContext string) (bool, error)
	InstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error)
}

type Impl struct {
}

// Checks if radius is installed based on kubeContext
func (i *Impl) CheckRadiusInstall(kubeContext string) (bool, error) {
	return CheckRadiusInstall(kubeContext)
}

// Installs radius on a cluster based on kubeContext
func (i *Impl) InstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error) {
	return Install(ctx, clusterOptions, kubeContext)
}
