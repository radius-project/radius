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
	"context"
	"errors"
	"fmt"
	"strings"

	helmaction "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/Masterminds/semver"
	"github.com/radius-project/radius/pkg/version"
)

const (
	ContourChartDefaultVersion = "11.1.1"
	DaprChartDefaultVersion    = "1.14.4"
)

type CLIClusterOptions struct {
	Radius ChartOptions
}

type ClusterOptions struct {
	Contour ContourOptions
	Radius  ChartOptions
	Dapr    ChartOptions
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
		Contour: ContourOptions{
			ChartVersion: ContourChartDefaultVersion,
		},
		Radius: ChartOptions{
			ChartVersion: chartVersion,
			Namespace:    RadiusSystemNamespace,
			ReleaseName:  radiusReleaseName,
			ChartRepo:    radiusHelmRepo,
		},
		Dapr: ChartOptions{
			ChartVersion: DaprChartDefaultVersion,
			Namespace:    DaprSystemNamespace,
			ReleaseName:  daprReleaseName,
			ChartRepo:    daprHelmRepo,
		},
	}
}

// PopulateDefaultClusterOptions compares the CLI options provided by the user to the default options and returns a
// ClusterOptions object with the CLI options overriding the default options if they are provided.
func PopulateDefaultClusterOptions(cliOptions CLIClusterOptions) ClusterOptions {
	options := NewDefaultClusterOptions()

	// If any of the CLI options are provided, override the default options.
	if cliOptions.Radius.Reinstall {
		options.Radius.Reinstall = cliOptions.Radius.Reinstall
	}

	if cliOptions.Radius.ChartPath != "" {
		options.Radius.ChartPath = cliOptions.Radius.ChartPath
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

	return options
}

// Install takes in a context, clusterOptions and kubeContext and returns a boolean and an error. If an
// error is encountered, it is returned.
func Install(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error) {
	// Do note: the namespace passed in to rad install kubernetes
	// doesn't match the namespace where we deploy radius.
	// The RPs and other resources are all deployed to the
	// 'radius-system' namespace. The namespace passed in will be
	// where pods/services/deployments will be put for rad deploy.
	radiusFound, err := ApplyHelmChart(clusterOptions.Radius, kubeContext)
	if err != nil {
		return false, err
	}

	// Install Dapr
	daprFound, err := ApplyHelmChart(clusterOptions.Dapr, kubeContext)
	if err != nil {
		return false, err
	}

	err = ApplyContourHelmChart(clusterOptions.Contour, kubeContext)
	if err != nil {
		return false, err
	}
	// If both Radius and Dapr are installed, return true
	if radiusFound && daprFound {
		return true, err
	} else {
		return false, err
	}
}

// UninstallOnCluster retrieves the Helm configuration and runs the Contour and Radius Helm uninstall commands to remove
// the Helm releases from the cluster.
func UninstallOnCluster(kubeContext string, clusterOptions ClusterOptions) error {
	var helmOutput strings.Builder

	flags := genericclioptions.ConfigFlags{
		Namespace: &clusterOptions.Radius.Namespace,
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

	// Uninstall Radius
	err = RunHelmUninstall(helmConf, clusterOptions.Radius)
	if err != nil {
		return err
	}

	// Uninstall Dapr
	daprFlags := genericclioptions.ConfigFlags{
		Namespace: &clusterOptions.Dapr.Namespace,
		Context:   &kubeContext,
	}

	daprHelmConf, err := HelmConfig(&helmOutput, &daprFlags)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}
	err = RunHelmUninstall(daprHelmConf, clusterOptions.Dapr)
	if err != nil {
		return err
	}

	return nil
}

// Upgrade takes in a context, clusterOptions and kubeContext and returns a boolean and an error. If an
// error is encountered, it is returned.
func Upgrade(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error) {
	err := UpgradeHelmChart(clusterOptions.Radius, kubeContext)
	if err != nil {
		return false, err
	}
	fmt.Println("Radius upgrade complete")

	// Upgrade Dapr
	err = UpgradeHelmChart(clusterOptions.Dapr, kubeContext)
	if err != nil {
		return false, err
	}
	fmt.Println("Dapr upgrade complete")

	err = UpgradeContourHelmChart(clusterOptions.Contour, kubeContext)
	if err != nil {
		return false, err
	}
	fmt.Println("Contour upgrade complete")

	// If all upgrades succeed, return true
	return true, err
}

// queryRelease checks to see if a release is deployed to a namespace for a given kubecontext.
// If the release is found, it returns true and the version of the release. If the release is not found, it returns false.
// If an error occurs, it returns an error.
func queryRelease(kubeContext, namespace, releaseName string) (bool, string, error) {
	var helmOutput strings.Builder

	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}

	helmConf, err := HelmConfig(&helmOutput, &flags)
	if err != nil {
		return false, "", fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}
	histClient := helmaction.NewHistory(helmConf)

	releases, err := histClient.Run(releaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return false, "", nil
	} else if err != nil {
		return false, "", err
	} else if len(releases) == 0 {
		return false, "", nil
	}

	var latestRelease *release.Release
	for _, release := range releases {
		if release.Info.Status == "deployed" {
			latestRelease = release
			break
		}
	}

	return true, latestRelease.Chart.Metadata.Version, nil
}

// CheckRadiusInstall checks if the Radius release is installed in the given kubeContext and returns an InstallState object
// with the version of the release if installed, or an error if an error occurs while checking.
func CheckRadiusInstall(kubeContext string) (InstallState, error) {
	// Check if Radius is installed
	radiusInstalled, radiusVersion, err := queryRelease(kubeContext, RadiusSystemNamespace, radiusReleaseName)
	if err != nil {
		return InstallState{}, err
	}

	// Check if Dapr is installed
	daprInstalled, daprVersion, err := queryRelease(kubeContext, DaprSystemNamespace, daprReleaseName)
	if err != nil {
		return InstallState{}, err
	}

	return InstallState{
		RadiusInstalled: radiusInstalled,
		RadiusVersion:   radiusVersion,
		DaprInstalled:   daprInstalled,
		DaprVersion:     daprVersion,
	}, nil
}

// InstallState represents the state of the Radius helm chart installation on a Kubernetes cluster.
type InstallState struct {
	// RadiusInstalled denotes whether the Radius helm chart is installed on the cluster.
	RadiusInstalled bool

	// RadiusVersion is the version of the Radius helm chart installed on the cluster. Will be blank if Radius is not installed.
	RadiusVersion string

	// DaprInstalled denotes whether the Dapr helm chart is installed on the cluster.
	DaprInstalled bool

	// DaprVersion is the version of the Dapr helm chart installed on the cluster. Will be blank if Dapr is not installed.
	DaprVersion string
}

//go:generate mockgen -typed -destination=./mock_cluster.go -package=helm -self_package github.com/radius-project/radius/pkg/cli/helm github.com/radius-project/radius/pkg/cli/helm Interface

// Interface provides an abstraction over Helm operations for installing Radius.
type Interface interface {
	// CheckRadiusInstall checks whether Radius is installed on the cluster, based on the specified Kubernetes context.
	CheckRadiusInstall(kubeContext string) (InstallState, error)

	// InstallRadius installs Radius on the cluster, based on the specified Kubernetes context.
	InstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error)

	// UninstallRadius uninstalls Radius from the cluster based on the specified Kubernetes context. Will succeed regardless of whether Radius is installed.
	UninstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error

	// UpgradeRadius upgrades Radius on the cluster, based on the specified Kubernetes context.
	UpgradeRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error)
}

type Impl struct {
}

// Checks if radius is installed based on kubeContext.
func (i *Impl) CheckRadiusInstall(kubeContext string) (InstallState, error) {
	return CheckRadiusInstall(kubeContext)
}

// Installs radius on a cluster based on kubeContext.
func (i *Impl) InstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error) {
	return Install(ctx, clusterOptions, kubeContext)
}

// UninstallRadius uninstalls RADIUS from the specified Kubernetes cluster, and returns an error if it fails.
func (i *Impl) UninstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) error {
	return UninstallOnCluster(kubeContext, clusterOptions)
}

// UpgradeRadius upgrades the Radius installation on the cluster, based on the specified Kubernetes context.
func (i *Impl) UpgradeRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error) {
	return Upgrade(ctx, clusterOptions, kubeContext)
}
