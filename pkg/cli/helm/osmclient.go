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
	// "github.com/project-radius/radius/pkg/cli/azure"
	// "helm.sh/helm/v3/pkg/chart/loader"
)

const (
	osmReleaseName = "osm"
	osmHelmRepo    = "https://openservicemesh.github.io/osm"
)

// currently we only have the chartversion option
type OsmOptions struct {
	ChartVersion string
}

func ApplyOsmHelmChart(options OsmOptions) error {
	var helmOutput strings.Builder

	helmConf, err := HelmConfig(RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	var helmChart *chart.Chart
	// ChartPath is not one of the osmoptions, so we will just retrieve the chart from the container registry
	helmChart, err = helmChartFromContainerRegistry(options.ChartVersion, helmConf, osmHelmRepo, osmReleaseName)

	if err != nil {
		return fmt.Errorf("failed to load helm chart, err: %w, helm output: %s", err, helmOutput.String())
	}

	// Retrieve the history
	histClient := helm.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists

	//Inokve the installation of osm control plane

	//retrieve the history of the releases
	_, err = histClient.Run(radiusReleaseName)
	//if a previous release is not found
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("Installing new OSM Kubernetes environment to namespace: %s", RadiusSystemNamespace)

		//Installation of osm
		err = runOsmHelmInstall(helmConf, helmChart)
		if err != nil {
			return fmt.Errorf("failed to run osm helm install, err: %w, helm output: %s", err, helmOutput.String())
		}
	} else if err == nil {
		output.LogInfo("Found existing OSM Kubernetes installation")
	}
	return err

}

func runOsmHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.Namespace = RadiusSystemNamespace
	installClient.ReleaseName = osmReleaseName
	installClient.Wait = true
	installClient.Timeout = installTimeout
	return runInstall(installClient, helmChart)
}

func RunOsmHelmUninstall(helmConf *helm.Configuration) error {
	output.LogInfo("Uninstalling OSM from namespace: %s", RadiusSystemNamespace)
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = uninstallTimeout
	uninstallClient.Wait = true
	_, err := uninstallClient.Run(osmReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("OSM not found")
		return nil
	}
	return err
}
