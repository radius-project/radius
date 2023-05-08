/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

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

func RunContourHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = contourReleaseName
	installClient.Namespace = RadiusSystemNamespace
	installClient.CreateNamespace = true

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
