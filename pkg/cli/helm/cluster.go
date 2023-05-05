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
	ContourChartDefaultVersion = "11.1.1"
)

type CLIClusterOptions struct {
	Radius RadiusOptions
}

type ClusterOptions struct {
	Contour ContourOptions
	Radius  RadiusOptions
}

// # Function Explanation
// 
//	NewDefaultClusterOptions() sets the default values for the ClusterOptions struct, such as the chart version and tag, 
//	based on the version of the CLI. If the version is an edge build, the latest available version is used. If an error 
//	occurs, the function will return an empty ClusterOptions struct.
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

// # Function Explanation
// 
//	PopulateDefaultClusterOptions takes in a CLIClusterOptions object and returns a ClusterOptions object with the default 
//	values, unless any of the CLI options are provided, in which case they will override the default values. If any of the 
//	CLI options are invalid, an error will be returned.
func PopulateDefaultClusterOptions(cliOptions CLIClusterOptions) ClusterOptions {
	options := NewDefaultClusterOptions()

	// If any of the CLI options are provided, override the default options.
	if cliOptions.Radius.Reinstall {
		options.Radius.Reinstall = cliOptions.Radius.Reinstall
	}

	if cliOptions.Radius.ChartPath != "" {
		options.Radius.ChartPath = cliOptions.Radius.ChartPath
	}

	if len(cliOptions.Radius.Values) > 0 {
		options.Radius.Values = cliOptions.Radius.Values
	}
	return options
}

// Installs radius based on kubecontext in "radius-system" namespace
//
// # Function Explanation
// 
//	Install attempts to install the Radius control plane on a given cluster using the provided context and cluster options. 
//	It returns a boolean indicating whether an existing installation was found, and an error if one occurred during the 
//	installation process.
func Install(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error) {
	step := output.BeginStep("Installing Radius version %s control plane...", version.Version())
	foundExisting, err := InstallOnCluster(ctx, clusterOptions, kubeContext)
	if err != nil {
		return false, err
	}

	output.CompleteStep(step)
	return foundExisting, nil
}

// # Function Explanation
// 
//	InstallOnCluster applies the Helm charts for Radius and Contour to the cluster specified in the ClusterOptions 
//	parameter, using the kubeContext provided. It returns a boolean indicating whether an existing installation was found, 
//	and an error if one occurred.
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

// # Function Explanation
// 
//	UninstallOnCluster is a function that uninstalls the Radius and Contour Helm charts from a Kubernetes cluster. It takes 
//	in a kubeContext string and returns an error if any of the Helm uninstall commands fail. If an error is encountered, the
//	 function will return the Helm output along with the error.
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
//
// # Function Explanation
// 
//	CheckRadiusInstall checks if the Radius system is installed in the given Kubernetes context and returns a boolean 
//	indicating the result. It handles errors by returning false and an error if the Helm configuration fails, or an error 
//	other than "release not found" if the Helm history run fails.
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
//
// # Function Explanation
// 
//	The CheckRadiusInstall function checks if the Radius server is installed in the given Kubernetes context and returns a 
//	boolean value indicating the result, along with an error if one occurs.
func (i *Impl) CheckRadiusInstall(kubeContext string) (bool, error) {
	return CheckRadiusInstall(kubeContext)
}

// Installs radius on a cluster based on kubeContext
//
// # Function Explanation
// 
//	The InstallRadius function installs a Radius server on a Kubernetes cluster using the provided ClusterOptions and 
//	kubeContext. It returns a boolean indicating success or failure and an error if one occurs. Callers should check the 
//	boolean and handle any errors returned.
func (i *Impl) InstallRadius(ctx context.Context, clusterOptions ClusterOptions, kubeContext string) (bool, error) {
	return Install(ctx, clusterOptions, kubeContext)
}
