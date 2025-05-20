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
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	contourHelmRepo            = "https://charts.bitnami.com/bitnami"
	contourReleaseName         = "contour"
	ContourChartDefaultVersion = "11.1.1"
)

type ContourChartOptions struct {
	ChartOptions
	// HostNetwork specifies whether to use host networking for the Envoy pod.
	HostNetwork bool
	// Wait specifies whether to wait for the chart to be ready.
	Wait bool
}

// prepareContourChart prepares the Helm chart for Contour.
func prepareContourChart(helmAction HelmAction, options ContourChartOptions, kubeContext string) (*chart.Chart, *action.Configuration, error) {
	var helmChart *chart.Chart

	flags := genericclioptions.ConfigFlags{
		Namespace: &options.Namespace,
		Context:   &kubeContext,
	}

	helmConf, err := initHelmConfig(&flags)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Helm config, err: %w", err)
	}

	if options.ChartPath == "" {
		chartRepo := options.ChartRepo
		if chartRepo == "" {
			chartRepo = contourHelmRepo
		}

		helmChart, err = helmAction.HelmChartFromContainerRegistry(options.ChartVersion, helmConf, chartRepo, options.ReleaseName)
	} else {
		helmChart, err = helmAction.LoadChart(options.ChartPath)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load Helm chart, err: %w", err)
	}

	err = addContourValues(helmChart, options)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add Contour values, err: %w", err)
	}

	return helmChart, helmConf, nil
}

// addContourValues adds values to the helm chart to enable host networking for the Envoy pod, and sets the default
// LoadBalancer service ports to 8080 and 8443 so that they don't conflict with Envoy while using Host Networking. It
// returns an error if any of the nodes in the chart values are not found.
func addContourValues(helmChart *chart.Chart, options ContourChartOptions) error {
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

	if options.HostNetwork {
		// https://projectcontour.io/docs/main/deploy-options/#host-networking
		// https://github.com/bitnami/charts/blob/7550513a4f491bb999f95027a7bfcc35ff076c33/bitnami/contour/values.yaml#L605
		envoyNode := values["envoy"].(map[string]any)
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
		if servicePortsNode == nil {
			return fmt.Errorf("envoy.service.ports node not found in chart values")
		}

		// Set the default LoadBalancer service ports to 8080 and 8443
		// so that they don't conflict with Envoy while using Host Networking.
		servicePortsNode["http"] = 8080
		servicePortsNode["https"] = 8443
	}

	return nil
}
