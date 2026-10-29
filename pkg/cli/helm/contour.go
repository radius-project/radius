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
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/cli/output"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
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

// contourPlan is a Contour chart and the values to install or upgrade it with, after the Gateway API CRD checks.
type contourPlan struct {
	chart  *chart.Chart
	conf   *helm.Configuration
	values map[string]any
}

// getContourRelease returns the Contour release, or nil when Contour is not installed.
func (i *Impl) getContourRelease(options ContourChartOptions, kubeContext string) (*releasev1.Release, error) {
	flags := genericclioptions.ConfigFlags{
		Namespace: &options.Namespace,
		Context:   &kubeContext,
	}

	helmConf, err := initHelmConfig(&flags)
	if err != nil {
		return nil, fmt.Errorf("failed to get Helm config, err: %w", err)
	}

	release, err := i.Helm.RunHelmGet(helmConf, options.ReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return release, nil
}

// planContourInstall plans the Contour install for InstallRadius. It returns nil when Contour is already installed and
// options.Reinstall is false, because ApplyHelmChart leaves such a release unchanged.
func (i *Impl) planContourInstall(ctx context.Context, helmAction HelmAction, options ContourChartOptions, kubeContext string) (*contourPlan, error) {
	existing, err := i.getContourRelease(options, kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to check Contour installation status: %w", err)
	}

	if existing != nil && !options.Reinstall {
		if len(options.SetArgs) > 0 || len(options.SetFileArgs) > 0 {
			output.LogInfo("Contour is already installed, so Radius does not change it or apply the Contour values.")
		}
		return nil, nil
	}

	// ApplyHelmChart reinstalls an existing release with Helm's ResetThenReuseValues semantics.
	return i.planContour(ctx, helmAction, options, kubeContext, installCommand, existing, true)
}

// planContourUpgrade plans the Contour upgrade for UpgradeRadius. It returns nil when Contour is not installed, because
// UpgradeRadius upgrades only an installed Contour release.
func (i *Impl) planContourUpgrade(ctx context.Context, helmAction HelmAction, options ContourChartOptions, kubeContext string, reuseValues bool) (*contourPlan, error) {
	existing, err := i.getContourRelease(options, kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to check Contour installation status: %w", err)
	}

	if existing == nil {
		if len(options.SetArgs) > 0 || len(options.SetFileArgs) > 0 {
			output.LogInfo("Contour is not installed, so Radius does not apply the Contour values.")
		}
		return nil, nil
	}

	return i.planContour(ctx, helmAction, options, kubeContext, upgradeCommand, existing, reuseValues)
}

// planContour prepares the Contour chart and its values, and checks that installing or upgrading Contour neither takes
// over nor deletes Gateway API CRDs and that Contour can use the CRDs it does not install. command is the rad command
// that the errors tell the user to run again. existing is the Contour release to upgrade, or nil to install Contour.
// reuseValues is true when Helm merges the values stored on existing beneath the new values.
func (i *Impl) planContour(ctx context.Context, helmAction HelmAction, options ContourChartOptions, kubeContext string, command string, existing *releasev1.Release, reuseValues bool) (*contourPlan, error) {
	helmChart, helmConf, values, err := prepareContourChart(ctx, helmAction, options, kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare Contour Helm chart, err: %w", err)
	}
	if helmChart.Metadata == nil {
		return nil, errors.New("failed to prepare Contour Helm chart, err: the chart has no metadata")
	}

	crds, err := i.gatewayAPICRDs(ctx, kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to list Gateway API CRDs, err: %w", err)
	}

	check := gatewayAPICRDCheck{
		releaseName:      options.ReleaseName,
		releaseNamespace: options.Namespace,
		contourVersion:   strings.TrimPrefix(helmChart.Metadata.AppVersion, "v"),
		crds:             crds,
		command:          command,
		resetValues:      existing != nil && !reuseValues,
	}
	if support, found := contourGatewayAPISupportByVersion[check.contourVersion]; found {
		check.support = &support
	}

	var reusedValues map[string]any
	if existing != nil {
		check.releaseCRDs, err = gatewayAPICRDsInManifest(existing.Manifest)
		if err != nil {
			return nil, fmt.Errorf("failed to read the Contour release manifest, err: %w", err)
		}

		if reuseValues {
			reusedValues = existing.Config
		}
	}

	// The user's --contour-set and --contour-set-file values take precedence over this default.
	check.setManageCRDsDefault(values)

	effectiveValues, err := effectiveContourValues(helmChart, values, reusedValues)
	if err != nil {
		return nil, fmt.Errorf("failed to compute Contour values, err: %w", err)
	}

	warning, err := check.validate(effectiveValues)
	if err != nil {
		return nil, err
	}
	if warning != "" {
		output.LogInfo("Warning: %s", warning)
	}

	return &contourPlan{chart: helmChart, conf: helmConf, values: values}, nil
}

// prepareContourChart prepares the Helm chart for Contour and returns the user-supplied
// values for this invocation. The returned values map is separate from helmChart.Values
// so that Helm's release storage records only the overrides we explicitly want to apply,
// allowing ResetThenReuseValues semantics to work correctly on upgrade. The values do not
// yet include the Gateway API CRD management default, which depends on the cluster; see
// planContour.
func prepareContourChart(ctx context.Context, helmAction HelmAction, options ContourChartOptions, kubeContext string) (*chart.Chart, *helm.Configuration, map[string]any, error) {
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
		helmChart, err = helmAction.HelmChartFromContainerRegistry(ctx, options.ChartVersion, helmConf, options.ChartRepo, options.ReleaseName)
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
// the supplied ContourChartOptions. It always sets the gateway configuration. When HostNetwork
// is enabled, it also returns overrides to enable host networking on the Envoy pod and remaps
// the LoadBalancer service ports to 8080/8443 so they don't conflict with Envoy. It then applies
// the user's --contour-set and --contour-set-file values, which take precedence over these
// defaults. The returned map contains ONLY the overrides; it does not mutate the chart's
// default values.
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

	if err := parseSetValuesInto(values, options.SetArgs, options.SetFileArgs); err != nil {
		return nil, err
	}

	return values, nil
}
