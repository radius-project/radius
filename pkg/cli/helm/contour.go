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

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	contourHelmRepo            = "https://projectcontour.github.io/helm-charts"
	contourReleaseName         = "contour"
	ContourChartDefaultVersion = "0.1.0"
)

type ContourChartOptions struct {
	ChartOptions
	// HostNetwork specifies whether to use host networking for the Envoy pod.
	HostNetwork bool
	// Wait specifies whether to wait for the chart to be ready.
	Wait bool
}

// prepareContourChart prepares the Helm chart for Contour and returns the user-supplied
// values for this invocation. The returned values map is separate from helmChart.Values
// so that Helm's release storage records only the overrides we explicitly want to apply,
// allowing ResetThenReuseValues semantics to work correctly on upgrade.
func prepareContourChart(helmAction HelmAction, options ContourChartOptions, kubeContext string) (*chart.Chart, *action.Configuration, map[string]any, error) {
	var helmChart *chart.Chart

	flags := genericclioptions.ConfigFlags{
		Namespace: &options.Namespace,
		Context:   &kubeContext,
	}

	helmConf, err := initHelmConfig(&flags)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get Helm config, err: %w", err)
	}

	if options.ChartPath == "" {
		helmChart, err = helmAction.HelmChartFromContainerRegistry(options.ChartVersion, helmConf, options.ChartRepo, options.ReleaseName)
	} else {
		helmChart, err = helmAction.LoadChart(options.ChartPath)
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load Helm chart, err: %w", err)
	}

	userValues, err := buildContourValues(options)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to build Contour values, err: %w", err)
	}

	return helmChart, helmConf, userValues, nil
}

// buildContourValues builds a fresh user-values map for the Contour Helm chart based on
// the supplied ContourChartOptions. It always sets the gateway configuration and Gateway API
// CRD management. When HostNetwork is enabled, it also returns overrides to enable host
// networking on the Envoy pod and remaps the LoadBalancer service ports to 8080/8443 so
// they don't conflict with Envoy. The returned map contains ONLY the overrides; it does
// not mutate the chart's default values.
//
// References:
//
//	https://projectcontour.io/docs/main/deploy-options/#host-networking
//	https://github.com/projectcontour/helm-charts/blob/81304159bb794a6d5ec874d1f29c696f63cff6ad/charts/contour/values.yaml#L962
func buildContourValues(options ContourChartOptions) (map[string]any, error) {
	values := map[string]any{}

	// Configure gateway reference for the default Contour gateway.
	values["configInline"] = map[string]any{
		"gateway": map[string]any{
			"gatewayRef": map[string]any{
				"name":      DefaultContourGatewayName,
				"namespace": DefaultContourGatewayNamespace,
			},
		},
	}

	// Enable Gateway API CRD management.
	values["gatewayAPI"] = map[string]any{
		"manageCRDs": true,
	}

	if options.HostNetwork {
		values["envoy"] = map[string]any{
			"hostNetwork": true,
			"dnsPolicy":   "ClusterFirstWithHostNet",
			// Container ports for the Envoy pod must be 80/443 to allow Envoy to bind
			// directly on the host network.
			"containerPorts": map[string]any{
				"http":  80,
				"https": 443,
			},
			// Move the default LoadBalancer service ports to 8080/8443 so they don't
			// conflict with Envoy while it is using host networking.
			"service": map[string]any{
				"ports": map[string]any{
					"http":  8080,
					"https": 8443,
				},
			},
		}
	}

	return values, nil
}
