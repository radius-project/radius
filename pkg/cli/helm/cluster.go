/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helm

import (
	context "context"
	"errors"
	"fmt"

	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/Masterminds/semver"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/version"
)

type CLIClusterOptions struct {
	Radius  ChartOptions
	Contour ChartOptions
}

type ClusterOptions struct {
	Radius  RadiusChartOptions
	Contour ContourChartOptions
}

// NewDefaultClusterOptions sets the default values for the ClusterOptions struct, using the chart version that matches
// the channel of the CLI (major.minor) or the latest available version if it is an edge build.
func NewDefaultClusterOptions() ClusterOptions {
	// By default we use the chart version that matches the channel of the CLI (major.minor)
	// If this is an edge build, we'll use the latest available.
	chartVersion := version.ChartVersion()
	if !version.IsEdgeChannel() {
		// When the chart version is the final release, we should use the ~ operator to ensure we fetch the latest patch version.
		// For the pre release, we should use the exact version.
		ver, _ := semver.NewVersion(chartVersion)
		preRelease := ver.Prerelease()
		if preRelease == "" {
			chartVersion = fmt.Sprintf("~%s", version.ChartVersion())
		}
	}

	return ClusterOptions{
		Radius: RadiusChartOptions{
			ChartOptions: ChartOptions{
				ChartVersion: chartVersion,
				Namespace:    RadiusSystemNamespace,
				ReleaseName:  radiusReleaseName,
				ChartRepo:    radiusHelmRepo,
				Wait:         true,
			},
		},
		Contour: ContourChartOptions{
			ChartOptions: ChartOptions{
				ChartVersion: ContourChartDefaultVersion,
				Namespace:    RadiusSystemNamespace,
				ReleaseName:  contourReleaseName,
				ChartRepo:    contourHelmRepo,
				Wait:         false,
			},
			HostNetwork: false,
		},
	}
}

// PopulateDefaultClusterOptions compares the CLI options provided by the user to the default options and returns a
// ClusterOptions object with the CLI options overriding the default options if they are provided.
func PopulateDefaultClusterOptions(cliOptions CLIClusterOptions) ClusterOptions {
	options := NewDefaultClusterOptions()

	// If any of the Radius CLI options are provided, override the default options.
	if cliOptions.Radius.Reinstall {
		options.Radius.Reinstall = cliOptions.Radius.Reinstall
	}

	if cliOptions.Radius.ChartPath != "" {
		options.Radius.ChartPath = cliOptions.Radius.ChartPath
	}

	if cliOptions.Radius.ChartRepo != "" {
		options.Radius.ChartRepo = cliOptions.Radius.ChartRepo
	}

	if len(cliOptions.Radius.SetArgs) > 0 {
		options.Radius.SetArgs = cliOptions.Radius.SetArgs
	}

	if len(cliOptions.Radius.SetFileArgs) > 0 {
		options.Radius.SetFileArgs = cliOptions.Radius.SetFileArgs
	}

	if cliOptions.Radius.ChartVersion != "" {
		options.Radius.ChartVersion = cliOptions.Radius.ChartVersion
	}

	if cliOptions.Radius.ChartRepo != "" {
		options.Radius.ChartRepo = cliOptions.Radius.ChartRepo
	}

	// Apply Contour overrides
	if cliOptions.Contour.Disabled {
		options.Contour.Disabled = cliOptions.Contour.Disabled
	}

	if cliOptions.Contour.ChartVersion != "" {
		options.Contour.ChartVersion = cliOptions.Contour.ChartVersion
	}

	if cliOptions.Contour.ChartRepo != "" {
		options.Contour.ChartRepo = cliOptions.Contour.ChartRepo
	}

	if cliOptions.Contour.ChartPath != "" {
		options.Contour.ChartPath = cliOptions.Contour.ChartPath
	}

	if len(cliOptions.Contour.SetArgs) > 0 {
		options.Contour.SetArgs = cliOptions.Contour.SetArgs
	}

	if len(cliOptions.Contour.SetFileArgs) > 0 {
		options.Contour.SetFileArgs = cliOptions.Contour.SetFileArgs
	}

	return options
}

// InstallState represents the state of the Radius installation on a Kubernetes cluster.
type InstallState struct {
	// RadiusInstalled denotes whether the Radius helm chart is installed on the cluster.
	RadiusInstalled bool

	// RadiusVersion is the version of the Radius helm chart installed on the cluster. Will be blank if Radius is not installed.
	RadiusVersion string

	// ContourInstalled denotes whether the Contour helm chart is installed on the cluster.
	ContourInstalled bool

	// ContourVersion is the version of the Contour helm chart installed on the cluster. Will be blank if Contour is not installed.
	ContourVersion string
}

//go:generate mockgen -typed -destination=./mock_cluster.go -package=helm -self_package github.com/radius-project/radius/pkg/cli/helm github.com/radius-project/radius/pkg/cli/helm Interface

// Interface provides an abstraction over Helm operations for installing Radius.
type Interface interface {
	// InstallRadius installs Radius on the cluster, based on the specified Kubernetes context.
	InstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error

	// UninstallRadius uninstalls Radius from the cluster based on the specified Kubernetes context. Will succeed regardless of whether Radius is installed.
	UninstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error

	// UpgradeRadius upgrades the Radius installation on the cluster, based on the specified Kubernetes context.
	UpgradeRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error

	// CheckRadiusInstall checks whether Radius is installed on the cluster, based on the specified Kubernetes context.
	CheckRadiusInstall(kubeContext string) (InstallState, error)

	// GetLatestRadiusVersion gets the latest available version of the Radius chart from the Helm repository.
	GetLatestRadiusVersion(ctx context.Context) (string, error)
}

type Impl struct {
	// HelmClient is the Helm client used to interact with the Kubernetes cluster.
	Helm HelmClient
}

var _ Interface = &Impl{}

// InstallRadius installs Radius and its dependencies (Contour) on the cluster using the provided options.
func (i *Impl) InstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error {
	// Do note: the namespace passed in to rad install kubernetes
	// doesn't match the namespace where we deploy radius.
	// The RPs and other resources are all deployed to the
	// 'radius-system' namespace. The namespace passed in will be
	// where pods/services/deployments will be put for rad deploy.

	helmAction := NewHelmAction(i.Helm)

	// Install Radius
	radiusHelmChart, radiusHelmConf, err := prepareRadiusChart(helmAction, clusterOptions.Radius, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to prepare Radius Helm chart, err: %w", err)
	}
	err = helmAction.ApplyHelmChart(kubeContext, radiusHelmChart, radiusHelmConf, clusterOptions.Radius.ChartOptions)
	if err != nil {
		return fmt.Errorf("failed to apply Radius Helm chart, err: %w", err)
	}

	if clusterOptions.Contour.Disabled {
		output.LogInfo("Contour is disabled, skipping installation")
		return nil
	}

	// Install Contour
	output.LogInfo("Installing Contour...")
	contourHelmChart, contourHelmConf, err := prepareContourChart(helmAction, clusterOptions.Contour, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to prepare Contour Helm chart, err: %w", err)
	}

	err = helmAction.ApplyHelmChart(kubeContext, contourHelmChart, contourHelmConf, clusterOptions.Contour.ChartOptions)
	if err != nil {
		return fmt.Errorf("failed to apply Contour Helm chart, err: %w", err)
	}

	return nil
}

// UninstallRadius uninstalls Radius and its dependencies (Contour) from the cluster using the provided options.
func (i *Impl) UninstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error {
	// Uninstall Radius
	if err := i.uninstallHelmRelease("Radius", radiusReleaseName, clusterOptions.Radius.Namespace, kubeContext); err != nil {
		return err
	}

	// Uninstall Contour
	if err := i.uninstallHelmRelease("Contour", contourReleaseName, clusterOptions.Radius.Namespace, kubeContext); err != nil {
		return err
	}

	return nil
}

func (i *Impl) uninstallHelmRelease(componentName, releaseName, namespace, kubeContext string) error {
	output.LogInfo("Uninstalling %s...", componentName)

	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}

	helmConf, err := initHelmConfig(&flags)
	if err != nil {
		return fmt.Errorf("failed to get helm config for %s, err: %w", componentName, err)
	}

	_, err = i.Helm.RunHelmUninstall(helmConf, releaseName, namespace, true)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			output.LogInfo("%s not found", releaseName)
		} else {
			return fmt.Errorf("failed to uninstall %s, err: %w", componentName, err)
		}
	}

	return nil
}

// CheckRadiusInstall checks if the Radius release is installed in the given kubeContext and returns an InstallState object
// with the version of the release if installed, or an error if an error occurs while checking.
func (i *Impl) CheckRadiusInstall(kubeContext string) (InstallState, error) {
	clusterOptions := NewDefaultClusterOptions()
	helmAction := NewHelmAction(i.Helm)

	// Check if Radius is installed
	radiusInstalled, radiusVersion, err := helmAction.QueryRelease(kubeContext, clusterOptions.Radius.ReleaseName, clusterOptions.Radius.Namespace)
	if err != nil {
		return InstallState{}, err
	}

	// Check if Contour is installed
	contourInstalled, contourVersion, err := helmAction.QueryRelease(kubeContext, clusterOptions.Contour.ReleaseName, clusterOptions.Contour.Namespace)
	if err != nil {
		return InstallState{}, err
	}

	return InstallState{RadiusInstalled: radiusInstalled, RadiusVersion: radiusVersion, ContourInstalled: contourInstalled, ContourVersion: contourVersion}, nil
}

// UpgradeRadius upgrades the Radius installation on the cluster, based on the specified Kubernetes context.
func (i *Impl) UpgradeRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error {
	helmAction := NewHelmAction(i.Helm)

	output.LogInfo("Upgrading Radius...")
	radiusHelmChart, radiusHelmConf, err := prepareRadiusChart(helmAction, clusterOptions.Radius, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to prepare Radius Helm chart, err: %w", err)
	}

	_, err = i.Helm.RunHelmUpgrade(radiusHelmConf, radiusHelmChart, clusterOptions.Radius.ReleaseName, clusterOptions.Radius.Namespace, true)
	if err != nil {
		return fmt.Errorf("failed to upgrade Radius, err: %w", err)
	}
	output.LogInfo("Radius upgrade complete")

	output.LogInfo("Upgrading Contour...")
	contourHelmChart, contourHelmConf, err := prepareContourChart(helmAction, clusterOptions.Contour, kubeContext)
	if err != nil {
		return fmt.Errorf("failed to prepare Contour Helm chart, err: %w", err)
	}
	_, err = i.Helm.RunHelmUpgrade(contourHelmConf, contourHelmChart, clusterOptions.Contour.ReleaseName, clusterOptions.Contour.Namespace, false)
	if err != nil {
		return fmt.Errorf("failed to upgrade Contour, err: %w", err)
	}
	output.LogInfo("Contour upgrade complete")

	return nil
}

// GetLatestRadiusVersion gets the latest available version of the Radius chart from the Helm repository.
func (i *Impl) GetLatestRadiusVersion(ctx context.Context) (string, error) {
	helmAction := NewHelmAction(i.Helm)

	// Use the same repository configuration as we use for installation
	clusterOptions := NewDefaultClusterOptions()
	chartRepo := clusterOptions.Radius.ChartRepo
	chartName := radiusReleaseName

	// Create a minimal helm configuration for chart operations
	flags := genericclioptions.ConfigFlags{
		Namespace: &clusterOptions.Radius.Namespace,
	}
	helmConf, err := initHelmConfig(&flags)
	if err != nil {
		return "", fmt.Errorf("failed to initialize helm config: %w", err)
	}

	// Use the existing HelmChartFromContainerRegistry method with empty version
	// to fetch the latest chart and extract its version
	chart, err := helmAction.HelmChartFromContainerRegistry("", helmConf, chartRepo, chartName)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest version from repository: %w", err)
	}

	// Extract version from chart metadata
	if chart.Metadata == nil || chart.Metadata.Version == "" {
		return "", fmt.Errorf("chart metadata does not contain version information")
	}

	return chart.Metadata.Version, nil
}
