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
	"k8s.io/cli-runtime/pkg/genericclioptions"
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

	namespace := RadiusSystemNamespace
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
	}
	helmConf, err := HelmConfig(helmOutput, &flags)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	// ChartPath is not one of the OSMoptions, so we will just retrieve the chart from the container registry
	helmChart, err := helmChartFromContainerRegistry(options.ChartVersion, helmConf, OSMHelmRepo, OSMReleaseName)
	if err != nil {
		return fmt.Errorf("failed to load helm chart, err: %w, helm output: %s", err, helmOutput.String())
	}

	err = modifyOSMResources(helmChart)
	if err != nil {
		return fmt.Errorf("Unable to modify the Open Service Mesh's resources")
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
	var helmOutput strings.Builder
	fmt.Println("Installed reached")
	installClient := helm.NewInstall(helmConf)
	fmt.Println("Install client created!:)")
	installClient.Namespace = RadiusSystemNamespace
	installClient.ReleaseName = OSMReleaseName
	installClient.Wait = true
	installClient.Timeout = installTimeout
	fmt.Println("Time to run install! Hopefully it works!")
	// , err := installClient.Run(helmChart, helmChart.Values)
	err := runInstall(installClient, helmChart)
	fmt.Println("Did it work? ")
	if err != nil {
		fmt.Errorf("OSM installation failed, err: %w, helm output: %s", err, helmOutput.String())
	}
	// if err != nil {
	// 	upgradeClient := helm.NewUpgrade(helmConf)
	// 	// upgradeClient.Install = true
	// 	upgradeClient.Wait = true
	// 	upgradeClient.Timeout = installTimeout
	// 	upgradeClient.Namespace = RadiusSystemNamespace
	// 	modification := map[string]interface{}{
	// 		"OpenServiceMesh": map[string]interface{}{
	// 			"install": true,
	// 		},
	// 	}
	// 	helmChart.Values = MergeMaps(helmChart.Values, modification)
	// 	_, err := upgradeClient.Run(OSMReleaseName, helmChart, helmChart.Values)
	// 	return err
	// }
	return err
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

func modifyOSMResources(helmChart *chart.Chart) error {
	values := helmChart.Values

	// A Map that alters the resource requests and limits of OSM pods
	modification := map[string]interface{}{
		"OpenServiceMesh": map[string]interface{}{
			"enablePermissiveTrafficPolicy": true,
			"osmNamespace":                  RadiusSystemNamespace,
			"controllerLogLevel":            "debug",
			"injector": map[string]interface{}{
				"resource": map[string]interface{}{
					"limits": map[string]interface{}{
						"cpu":    "100m",
						"memory": "16M",
					},
					"requests": map[string]interface{}{
						"cpu":    "100m",
						"memory": "16M",
					},
				},
			},
			"osmController": map[string]interface{}{
				"resource": map[string]interface{}{
					"limits": map[string]interface{}{
						"cpu":    "750m",
						"memory": "512M",
					},
					"requests": map[string]interface{}{
						"cpu":    "200m",
						"memory": "32M",
					},
				},
			},
			"osmBootstrap": map[string]interface{}{
				"resource": map[string]interface{}{
					"limits": map[string]interface{}{
						"cpu":    "200m",
						"memory": "32M",
					},
					"requests": map[string]interface{}{
						"cpu":    "100m",
						"memory": "32M",
					},
				},
			},
		},
	}

	// merging the modifications into the values map
	values = MergeMaps(values, modification)
	helmChart.Values = values
	return nil
}

func MergeMaps(a, b map[string]interface{}) map[string]interface{} {
	// b merges into a
	// This function is retrieved from the helm Github repo
	// https://github.com/helm/helm/blob/v3.9.0/pkg/cli/values/options.go#L91
	out := make(map[string]interface{}, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					out[k] = MergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}
