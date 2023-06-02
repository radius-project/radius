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
	"strings"

	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/version"
)

const (
	radiusReleaseName     = "radius"
	radiusHelmRepo        = "https://radius.azurecr.io/helm/v1/repo"
	RadiusSystemNamespace = "radius-system"
)

type RadiusOptions struct {
	Reinstall    bool
	ChartPath    string
	ChartVersion string
	ImageVersion string
	Values       string
}

// Apply the radius helm chart.
func ApplyRadiusHelmChart(options RadiusOptions, kubeContext string) (bool, error) {
	// For capturing output from helm.
	var helmOutput strings.Builder
	alreadyInstalled := false
	namespace := RadiusSystemNamespace
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}

	helmConf, err := HelmConfig(&helmOutput, &flags)
	if err != nil {
		return false, fmt.Errorf("failed to get Helm config, err: %w, Helm output: %s", err, helmOutput.String())
	}

	var helmChart *chart.Chart
	if options.ChartPath == "" {
		helmChart, err = helmChartFromContainerRegistry(options.ChartVersion, helmConf, radiusHelmRepo, radiusReleaseName)
	} else {
		helmChart, err = loader.Load(options.ChartPath)
	}

	if err != nil {
		return false, fmt.Errorf("failed to load Helm chart, err: %w, Helm output: %s", err, helmOutput.String())
	}

	err = AddRadiusValues(helmChart, &options)
	if err != nil {
		return false, fmt.Errorf("failed to add Radius values, err: %w, Helm output: %s", err, helmOutput.String())
	}

	// https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you
	// TODO: Apply CRDs because Helm doesn't upgrade CRDs for you.
	// https://github.com/project-radius/radius/issues/712
	// We need the CRDs to be public to do this (or consider unpacking the chart
	// for the CRDs)

	histClient := helm.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists
	version := version.Version()

	// See: https://github.com/helm/helm/blob/281380f31ccb8eb0c86c84daf8bcbbd2f82dc820/cmd/helm/upgrade.go#L99
	// The upgrade client's install option doesn't seem to work, so we have to check the history of releases manually
	// and invoke the install client.
	_, err = histClient.Run(radiusReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("Installing Radius version %s control plane to namespace: %s", version, RadiusSystemNamespace)

		err = runRadiusHelmInstall(helmConf, helmChart)
		if err != nil {
			return false, fmt.Errorf("failed to run Radius Helm install, err: \n%w\nHelm output:\n%s", err, helmOutput.String())
		}
	} else if options.Reinstall {
		output.LogInfo("Reinstalling Radius version %s control plane to namespace: %s", version, RadiusSystemNamespace)

		err = runRadiusHelmUpgrade(helmConf, radiusReleaseName, helmChart)
		if err != nil {
			return false, fmt.Errorf("failed to run Radius Helm upgrade, err: \n%w\nHelm output:\n%s", err, helmOutput.String())
		}
	} else if err == nil {
		alreadyInstalled = true
		output.LogInfo("Found existing Radius installation. Use '--reinstall' to force reinstallation.")
	}
	return alreadyInstalled, err
}

// AddRadiusValues adds values to the helm chart. It overrides the default values in following order:
// 1. lowest priority: Values from the helm chart default values.yaml
// 2. next priority: set correct image tag, potentially overwriting "latest" tag from step 1
// 3. highest priority: Values by the --set flag potentially overwriting values from step 1 and 2
func AddRadiusValues(helmChart *chart.Chart, options *RadiusOptions) error {
	values := helmChart.Values

	services := []string{"rp", "ucp", "de"}
	for _, service := range services {
		if _, ok := values[service]; !ok {
			values[service] = map[string]any{}
		}
		o := values[service].(map[string]any)
		o["tag"] = options.ImageVersion
	}

	err := strvals.ParseInto(options.Values, values)
	if err != nil {
		return err
	}

	return nil
}

func runRadiusHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = radiusReleaseName
	installClient.Namespace = RadiusSystemNamespace
	installClient.CreateNamespace = true
	installClient.Wait = true
	installClient.Timeout = installTimeout
	return runInstall(installClient, helmChart)
}

func runRadiusHelmUpgrade(helmConf *helm.Configuration, releaseName string, helmChart *chart.Chart) error {
	installClient := helm.NewUpgrade(helmConf)
	installClient.Namespace = RadiusSystemNamespace
	installClient.Wait = true
	installClient.Timeout = installTimeout
	installClient.Recreate = true //force recreating radius pods on adding or modfying azprovider
	return runUpgrade(installClient, releaseName, helmChart)
}

func RunRadiusHelmUninstall(helmConf *helm.Configuration) error {
	output.LogInfo("Uninstalling Radius from namespace: %s", RadiusSystemNamespace)
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = uninstallTimeout
	uninstallClient.Wait = true
	_, err := uninstallClient.Run(radiusReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("Radius not found")
		return nil
	}
	return err
}
