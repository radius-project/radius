// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	_ "embed"
	"fmt"
	"strings"

	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/storage/driver"
)

const (
	daprReleaseName = "dapr"
	daprHelmRepo    = "https://dapr.github.io/helm-charts/"
)

func ApplyDaprHelmChart(version string) error {
	// For capturing output from helm.
	var helmOutput strings.Builder

	helmConf, err := HelmConfig(RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	helmChart, err := helmChartFromRepo(version, helmConf, daprHelmRepo, daprReleaseName)
	if err != nil {
		return fmt.Errorf("failed to get dapr chart, err: %w, helm output: %s", err, helmOutput.String())
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
	_, err = histClient.Run(daprReleaseName)
	if err == driver.ErrReleaseNotFound {

		err = runDaprHelmInstall(helmConf, helmChart)
		if err != nil {
			return fmt.Errorf("failed to run helm install, err: %w, helm output: %s", err, helmOutput.String())
		}
	}

	return err
}

func runDaprHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = daprReleaseName
	installClient.Namespace = RadiusSystemNamespace

	_, err := installClient.Run(helmChart, helmChart.Values)
	return err
}

func RunDaprHelmUninstall(helmConf *helm.Configuration) error {
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = timeout
	uninstallClient.Wait = true
	_, err := uninstallClient.Run(daprReleaseName)
	return err
}
