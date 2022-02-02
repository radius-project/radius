// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/storage/driver"
)

const (
	radiusReleaseName     = "radius"
	radiusHelmRepo        = "https://radius.azurecr.io/helm/v1/repo"
	RadiusSystemNamespace = "radius-system"
)

func ApplyRadiusHelmChart(chartPath string, chartVersion string, containerImage string, containerTag string) error {
	// For capturing output from helm.
	var helmOutput strings.Builder

	helmConf, err := HelmConfig(RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	var helmChart *chart.Chart
	if chartPath == "" {
		helmChart, err = helmChartFromRepo(chartVersion, helmConf, radiusHelmRepo, radiusReleaseName)
	} else {
		helmChart, err = loader.Load(chartPath)
	}

	if err != nil {
		return fmt.Errorf("failed to load helm chart, err: %w, helm output: %s", err, helmOutput.String())
	}

	err = addRadiusValues(helmChart, containerImage, containerTag)

	if err != nil {
		return fmt.Errorf("failed to add radius values, err: %w, helm output: %s", err, helmOutput.String())
	}

	// https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you
	// TODO: Apply CRDs because Helm doesn't upgrade CRDs for you.
	// https://github.com/project-radius/radius/issues/712
	// We need the CRDs to be public to do this (or consider unpacking the chart
	// for the CRDs)

	histClient := helm.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists

	// See: https://github.com/helm/helm/blob/281380f31ccb8eb0c86c84daf8bcbbd2f82dc820/cmd/helm/upgrade.go#L99
	// The upgrade client's install option doesn't seem to work, so we have to check the history of releases manually
	// and invoke the install client.
	_, err = histClient.Run(radiusReleaseName)
	if err == driver.ErrReleaseNotFound {
		output.LogInfo("Installing new Radius Kubernetes environment to namespace: %s", RadiusSystemNamespace)

		err = runRadiusHelmInstall(helmConf, helmChart)
		if err != nil {
			return fmt.Errorf("failed to run helm install, err: %w, helm output: %s", err, helmOutput.String())
		}
	} else if err == nil {
		output.LogInfo("Found existing Radius Kubernetes environment")
	}

	return err
}

func runRadiusHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = radiusReleaseName
	installClient.Namespace = RadiusSystemNamespace
	_, err := installClient.Run(helmChart, helmChart.Values)
	return err
}

func addRadiusValues(helmChart *chart.Chart, containerImage string, containerTag string) error {
	values := helmChart.Values

	if containerImage != "" {
		values["container"] = containerImage
	}

	if containerTag != "" {
		values["tag"] = containerTag
	}

	return nil
}
func RunRadiusHelmUninstall(helmConf *helm.Configuration) error {
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = timeout
	_, err := uninstallClient.Run(radiusReleaseName)
	return err
}
