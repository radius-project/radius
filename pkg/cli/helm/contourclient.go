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

	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/project-radius/radius/pkg/cli/output"
)

const (
	contourHelmRepo    = "https://charts.bitnami.com/bitnami"
	contourReleaseName = "contour"
)

type ContourOptions struct {
	ChartVersion string
	HostNetwork  bool
}

// # Function Explanation
// 
//	ApplyContourHelmChart checks if a Contour Helm chart has been installed, and if not, installs it with the given 
//	ContourOptions. If an error occurs, it returns an error with the Helm output included.
func ApplyContourHelmChart(options ContourOptions, kubeContext string) error {
	// For capturing output from helm.
	var helmOutput strings.Builder

	namespace := RadiusSystemNamespace
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
	}

	helmConf, err := HelmConfig(&helmOutput, &flags)
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

	err = AddContourValues(helmChart, options)
	if err != nil {
		return err
	}

	// See: https://github.com/helm/helm/blob/281380f31ccb8eb0c86c84daf8bcbbd2f82dc820/cmd/helm/upgrade.go#L99
	// The upgrade client's install option doesn't seem to work, so we have to check the history of releases manually
	// and invoke the install client.
	_, err = histClient.Run(contourReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		err = RunContourHelmInstall(helmConf, helmChart)
		if err != nil {
			return fmt.Errorf("failed to run contour helm install, err: %w, helm output: %s", err, helmOutput.String())
		}
	}

	return err
}

// # Function Explanation
// 
//	AddContourValues configures the chart values for Contour to use Host Networking if the option is set, and sets the 
//	container and service ports to avoid conflicts. It returns an error if any of the nodes in the chart values are not 
//	found.
func AddContourValues(helmChart *chart.Chart, options ContourOptions) error {
	if options.HostNetwork {
		// https://projectcontour.io/docs/main/deploy-options/#host-networking
		// https://github.com/bitnami/charts/blob/7550513a4f491bb999f95027a7bfcc35ff076c33/bitnami/contour/values.yaml#L605
		envoyNode := helmChart.Values["envoy"].(map[string]any)
		if envoyNode == nil {
			return fmt.Errorf("envoy node not found in chart values")
		}

		envoyNode["hostNetwork"] = true
		envoyNode["dnsPolicy"] = "ClusterFirstWithHostNet"

		containerPortsNode := envoyNode["containerPorts"].(map[string]any)
		if containerPortsNode == nil {
			return fmt.Errorf("envoy.containerPorts node not found in chart values")
		}

		// Sets the container ports for the Envoy pod. These need to be set to 80 and
		// 443 to allow Envoy to access the host network.
		containerPortsNode["http"] = 80
		containerPortsNode["https"] = 443

		serviceNode := envoyNode["service"].(map[string]any)
		if serviceNode == nil {
			return fmt.Errorf("envoy.service node not found in chart values")
		}

		servicePortsNode := serviceNode["ports"].(map[string]any)
		if serviceNode == nil {
			return fmt.Errorf("envoy.service.ports node not found in chart values")
		}

		// This is a hack that sets the default LoadBalancer service ports to 8080 and 8443
		// so that they don't conflict with Envoy while using Host Networking.
		servicePortsNode["http"] = 8080
		servicePortsNode["https"] = 8443
	}

	return nil
}

// # Function Explanation
// 
//	RunContourHelmInstall configures and runs an install of the Contour Helm chart using the provided Helm configuration and
//	 chart. It returns an error if the install fails.
func RunContourHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = contourReleaseName
	installClient.Namespace = RadiusSystemNamespace
	installClient.CreateNamespace = true

	return runInstall(installClient, helmChart)
}

// # Function Explanation
// 
//	RunContourHelmUninstall attempts to uninstall Contour from the specified namespace using the provided helm 
//	configuration. It returns an error if the uninstall fails, or nil if the Contour release was not found.
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
