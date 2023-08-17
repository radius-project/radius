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
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/project-radius/radius/pkg/version"
)

const (
	ContourChartDefaultVersion = "11.1.1"
)

type CLIClusterOptions struct {
	Radius RadiusOptions
}

type ClusterOptions struct {
	Contour ContourOptions
	Radius  RadiusOptions
}

// NewDefaultClusterOptions sets the default values for the ClusterOptions struct, using the chart version that matches
// the channel of the CLI (major.minor) or the latest available version if it is an edge build.
func NewDefaultClusterOptions() ClusterOptions {
	// By default we use the chart version that matches the channel of the CLI (major.minor)
	// If this is an edge build, we'll use the latest available.
	chartVersion := version.ChartVersion()
	if !version.IsEdgeChannel() {
		chartVersion = fmt.Sprintf("~%s", version.ChartVersion())
	}

	return ClusterOptions{
		Contour: ContourOptions{
			ChartVersion: ContourChartDefaultVersion,
		},
		Radius: RadiusOptions{
			ChartVersion: chartVersion,
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
	return options
}

// InstallRadius installs Radius on the cluster, based on the specified Kubernetes context.
//

// Install takes in a context, clusterOptions and kubeContext and returns a boolean and an error. If an
// error is encountered, it is returned.
func Install(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error) {
	foundExisting, err := InstallOnCluster(ctx, clusterOptions, kubeContext)
	if err != nil {
		return false, err
	}

	return foundExisting, nil
}

// InstallOnCluster applies the Helm charts for Radius and Contour to the cluster, and returns whether an existing
// installation was found. If an error occurs, it is returned.
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

// UninstallOnCluster retrieves the Helm configuration and runs the Contour and Radius Helm uninstall commands to remove
// the Helm releases from the cluster.
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

// CheckRadiusInstall checks whether Radius is installed on the cluster, based on the specified Kubernetes context.
//

// CheckRadiusInstall checks if the Radius release is installed in the given kubeContext and returns an InstallState object
// with the version of the release if installed, or an error if an error occurs while checking.
func CheckRadiusInstall(kubeContext string) (InstallState, error) {
	var helmOutput strings.Builder

	namespace := RadiusSystemNamespace
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}

	helmConf, err := HelmConfig(&helmOutput, &flags)
	if err != nil {
		return InstallState{}, fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}
	histClient := helmaction.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists

	releases, err := histClient.Run(radiusReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return InstallState{}, nil
	} else if err != nil {
		return InstallState{}, err
	} else if len(releases) == 0 {
		return InstallState{}, nil
	}

	version := releases[0].Chart.Metadata.Version
	return InstallState{Installed: true, Version: version}, nil
}

// InstallState represents the state of the Radius helm chart installation on a Kubernetes cluster.
type InstallState struct {
	// Installed denotes whether the Radius helm chart is installed on the cluster.
	Installed bool

	// Version is the version of the Radius helm chart installed on the cluster. Will be blank if Radius is not installed.
	Version string
}

//go:generate mockgen -destination=./mock_cluster.go -package=helm -self_package github.com/project-radius/radius/pkg/cli/helm github.com/project-radius/radius/pkg/cli/helm Interface

// Interface provides an abstraction over Helm operations for installing Radius.
type Interface interface {
	// CheckRadiusInstall checks whether Radius is installed on the cluster, based on the specified Kubernetes context.
	CheckRadiusInstall(kubeContext string) (InstallState, error)

	// InstallRadius installs Radius on the cluster, based on the specified Kubernetes context.
	InstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error)

	// UninstallRadius uninstalls Radius from the cluster based on the specified Kubernetes context. Will succeed regardless of whether Radius is installed.
	UninstallRadius(ctx context.Context, kubeContext string) error
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
func (i *Impl) UninstallRadius(ctx context.Context, kubeContext string) error {
	return UninstallOnCluster(kubeContext)
}
