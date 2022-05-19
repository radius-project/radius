// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/storage/driver"
)

const (
	OSMReleaseName = "osm"
	OSMHelmRepo    = "https://openservicemesh.github.io/osm"
)

// currently we only have the chartversion option
type OSMOptions struct {
	ChartVersion string
}

func ApplyOSMHelmChart(options OSMOptions) error {
	var helmOutput strings.Builder

	helmConf, err := HelmConfig(RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	// ChartPath is not one of the OSMoptions, so we will just retrieve the chart from the container registry
	helmChart, err := helmChartFromContainerRegistry(options.ChartVersion, helmConf, OSMHelmRepo, OSMReleaseName)

	if err != nil {
		return fmt.Errorf("failed to load helm chart, err: %w, helm output: %s", err, helmOutput.String())
	}

	// Retrieve the history
	histClient := helm.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists

	// Invoke the installation of OSM control plane

	// retrieve the history of the releases
	_, err = histClient.Run(OSMReleaseName)
	// if a previous release is not found
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("Installing Open Service Mesh (OSM) to namespace: %s", RadiusSystemNamespace)

		// Installation of OSM
		err = runOSMHelmInstall(helmConf, helmChart)
		if err != nil {
			return fmt.Errorf("failed to run Open Service Mesh (OSM) helm install, err: %w, helm output: %s", err, helmOutput.String())
		}
	} else if err == nil {
		output.LogInfo("Found existing Open Service Mesh (OSM) installation")
	}
	return err

}

func runOSMHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.Namespace = RadiusSystemNamespace
	installClient.ReleaseName = OSMReleaseName
	installClient.Wait = true
	installClient.Timeout = installTimeout
	return runInstall(installClient, helmChart)
}

func RunOSMHelmUninstall(helmConf *helm.Configuration) error {
	output.LogInfo("Uninstalling Open Service Mesh (OSM) from namespace: %s", RadiusSystemNamespace)
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = uninstallTimeout
	uninstallClient.Wait = true
	_, err := uninstallClient.Run(OSMReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("Open Service Mesh (OSM) not found")
		return nil
	}
	return err
}
