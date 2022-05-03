// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/storage/driver"
)

const (
	contourHelmRepo     = "https://charts.bitnami.com/bitnami"
	contourHelmRepoName = "bitnami"
	contourReleaseName  = "contour"
)

type ContourOptions struct {
	ChartVersion string
}

func ApplyContourHelmChart(options ContourOptions) error {
	// For capturing output from helm.
	var helmOutput strings.Builder

	helmConf, err := HelmConfig(RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	helmChart, err := helmChartFromContainerRegistry(options.ChartVersion, helmConf, contourHelmRepo, contourReleaseName)
	if err != nil {
		return fmt.Errorf("failed to get contour chart, err: %w, helm output: %s", err, helmOutput.String())
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
	_, err = histClient.Run(contourReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		err = RunContourHelmInstall(helmConf, helmChart)
		if err != nil {
			return fmt.Errorf("failed to run helm install, err: %w, helm output: %s", err, helmOutput.String())
		}
	}

	return err
}

func RunContourHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = contourReleaseName
	installClient.Namespace = RadiusSystemNamespace
	return runInstall(installClient, helmChart)
}

func RunContourHelmUninstall(helmConf *helm.Configuration) error {
	output.LogInfo("Uninstalling Contour from namespace: %s", RadiusSystemNamespace)
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = uninstallTimeout
	uninstallClient.Wait = true
	_, err := uninstallClient.Run(contourReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("Contour not found")
		return nil
	}
	return err
}
