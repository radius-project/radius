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
	haproxyReleaseName = "haproxy-ingress"
	haproxyHelmRepo    = "https://haproxy-ingress.github.io/charts"
)

type HAProxyOptions struct {
	// See: https://github.com/haproxy-ingress/charts/blob/2009202f2bfe045a8fcdb99e7880cdd54f2ad5bc/haproxy-ingress/values.yaml#L137
	UseHostNetwork bool
}

func ApplyHAProxyHelmChart(version string, options HAProxyOptions) error {
	// For capturing output from helm.
	var helmOutput strings.Builder

	helmConf, err := helmConfig(RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	helmChart, err := helmChartFromRepo(version, helmConf, haproxyHelmRepo, haproxyReleaseName)
	if err != nil {
		return fmt.Errorf("failed to get haproxy chart, err: %w, helm output: %s", err, helmOutput.String())
	}

	// https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you
	// TODO: Apply CRDs because Helm doesn't upgrade CRDs for you.
	// https://github.com/project-radius/radius/issues/712
	// We need the CRDs to be public to do this (or consider unpacking the chart
	// for the CRDs)

	histClient := helm.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists

	err = addHAProxyValues(helmChart, options)
	if err != nil {
		return err
	}

	// See: https://github.com/helm/helm/blob/281380f31ccb8eb0c86c84daf8bcbbd2f82dc820/cmd/helm/upgrade.go#L99
	// The upgrade client's install option doesn't seem to work, so we have to check the history of releases manually
	// and invoke the install client.
	_, err = histClient.Run(haproxyReleaseName)
	if err == driver.ErrReleaseNotFound {

		err = runHAProxyHelmInstall(helmConf, helmChart)
		if err != nil {
			return fmt.Errorf("failed to run helm install, err: %w, helm output: %s", err, helmOutput.String())
		}
	}

	return err
}

// Values for configuring the install of the helm chart
func addHAProxyValues(helmChart *chart.Chart, options HAProxyOptions) error {
	controllerNode := helmChart.Values["controller"].(map[string]interface{})
	if controllerNode == nil {
		return fmt.Errorf("controller node not found in chart values")
	}

	if options.UseHostNetwork {
		controllerNode["hostNetwork"] = true
	}

	extraArgsNode := controllerNode["extraArgs"].(map[string]interface{})
	if extraArgsNode == nil {
		return fmt.Errorf("extraArgs node not found in chart values")
	}

	extraArgsNode["watch-gateway"] = "true"
	return nil
}

func runHAProxyHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = haproxyReleaseName
	installClient.Namespace = RadiusSystemNamespace

	_, err := installClient.Run(helmChart, helmChart.Values)
	return err
}
