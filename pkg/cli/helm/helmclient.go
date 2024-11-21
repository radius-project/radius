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
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/radius-project/radius/pkg/cli/output"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	radiusReleaseName     = "radius"
	radiusHelmRepo        = "oci://ghcr.io/radius-project/helm-chart"
	RadiusSystemNamespace = "radius-system"

	daprReleaseName     = "dapr"
	daprHelmRepo        = "https://dapr.github.io/helm-charts"
	DaprSystemNamespace = "dapr-system"
)

// ChartOptions describes the options for the a helm chart.
type ChartOptions struct {
	// Target namespace for deployment
	Namespace string

	// ReleaseName specifies the release name for the helm chart.
	ReleaseName string

	// ChartRepo specifies the helm chart repository.
	ChartRepo string

	// Reinstall specifies whether to reinstall the chart (helm upgrade).
	Reinstall bool

	// ChartPath specifies an override for the chart location.
	ChartPath string

	// ChartVersion specifies the chart version.
	ChartVersion string

	// SetArgs specifies as set of additional "values" to pass to helm. These are specified using the command-line syntax accepted
	// by helm, in the order they appear on the command line (last one wins).
	SetArgs []string

	// SetFileArgs specifies as set of additional "values" from file to pass it to helm.
	SetFileArgs []string
}

// ApplyHelmChart checks if a Helm chart is already installed, and if not, installs it or upgrades it if the
// "Reinstall" option is set. It returns a boolean indicating if the chart was already installed and an error if one occurred.
func ApplyHelmChart(options ChartOptions, kubeContext string) (bool, error) {
	// For capturing output from helm.
	var helmOutput strings.Builder
	alreadyInstalled := false

	flags := genericclioptions.ConfigFlags{
		Namespace: &options.Namespace,
		Context:   &kubeContext,
	}

	helmConf, err := HelmConfig(&helmOutput, &flags)
	if err != nil {
		return false, fmt.Errorf("failed to get Helm config, err: %w, Helm output: %s", err, helmOutput.String())
	}

	var helmChart *chart.Chart
	if options.ChartPath == "" {
		helmChart, err = helmChartFromContainerRegistry(options.ChartVersion, helmConf, options.ChartRepo, options.ReleaseName)
	} else {
		helmChart, err = loader.Load(options.ChartPath)
	}

	if err != nil {
		return false, fmt.Errorf("failed to load Helm chart, err: %w, Helm output: %s", err, helmOutput.String())
	}

	err = AddValues(helmChart, &options)
	if err != nil {
		return false, fmt.Errorf("failed to add Radius values, err: %w, Helm output: %s", err, helmOutput.String())
	}

	// https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you
	// TODO: Apply CRDs because Helm doesn't upgrade CRDs for you.
	// https://github.com/radius-project/radius/issues/712
	// We need the CRDs to be public to do this (or consider unpacking the chart
	// for the CRDs)

	histClient := helm.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists

	// See: https://github.com/helm/helm/blob/281380f31ccb8eb0c86c84daf8bcbbd2f82dc820/cmd/helm/upgrade.go#L99
	// The upgrade client's install option doesn't seem to work, so we have to check the history of releases manually
	// and invoke the install client.
	_, err = histClient.Run(options.ReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		err = runHelmInstall(helmConf, helmChart, options)
		if err != nil {
			return false, fmt.Errorf("failed to run Radius Helm install, err: \n%w\nHelm output:\n%s", err, helmOutput.String())
		}
	} else if options.Reinstall {
		err = runHelmUpgrade(helmConf, options.ReleaseName, helmChart, options)
		if err != nil {
			return false, fmt.Errorf("failed to run Radius Helm upgrade, err: \n%w\nHelm output:\n%s", err, helmOutput.String())
		}
	} else if err == nil {
		alreadyInstalled = true
	}
	return alreadyInstalled, err
}

// AddValues parses the --set arguments in order and adds them to the helm chart values, returning an error if any of
// the arguments are invalid.
func AddValues(helmChart *chart.Chart, options *ChartOptions) error {
	values := helmChart.Values

	// Parse --set arguments in order so that the last one wins.
	for _, arg := range options.SetArgs {
		err := strvals.ParseInto(arg, values)
		if err != nil {
			return err
		}
	}

	for _, arg := range options.SetFileArgs {
		if runtime.GOOS == "windows" {
			arg = filepath.ToSlash(arg)
		}

		reader := func(rs []rune) (any, error) {
			data, err := os.ReadFile(string(rs))
			return string(data), err
		}

		err := strvals.ParseIntoFile(arg, values, reader)
		if err != nil {
			return err
		}
	}

	return nil
}

func runHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart, options ChartOptions) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = options.ReleaseName
	installClient.Namespace = options.Namespace
	installClient.CreateNamespace = true
	installClient.Wait = true
	installClient.Timeout = installTimeout
	return runInstall(installClient, helmChart)
}

func runHelmUpgrade(helmConf *helm.Configuration, releaseName string, helmChart *chart.Chart, options ChartOptions) error {
	installClient := helm.NewUpgrade(helmConf)
	installClient.Namespace = options.Namespace
	installClient.Wait = true
	installClient.Timeout = installTimeout
	installClient.Recreate = true //force recreating radius pods on adding or modfying azprovider
	return runUpgrade(installClient, options.ReleaseName, helmChart)
}

// RunRadiusHelmUninstall attempts to uninstall Radius from the Radius system namespace
// using a helm configuration, and returns an error if the uninstall fails.
func RunHelmUninstall(helmConf *helm.Configuration, options ChartOptions) error {
	output.LogInfo("Uninstalling %s from namespace: %s", options.ReleaseName, options.Namespace)
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = uninstallTimeout
	uninstallClient.Wait = true
	_, err := uninstallClient.Run(options.ReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("%s not found", options.ReleaseName)
		return nil
	}
	return err
}
