/*
Copyright 2025 The Radius Authors.

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
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	daprReleaseName         = "dapr"
	daprHelmRepo            = "https://dapr.github.io/helm-charts"
	DaprSystemNamespace     = "dapr-system"
	DaprChartDefaultVersion = "1.14.4"
)

type DaprChartOptions struct {
	ChartOptions
}

// TODO COMMENT
func prepareDaprChart(helmAction HelmAction, options DaprChartOptions, kubeContext string) (*chart.Chart, *action.Configuration, error) {
	helmOutput := strings.Builder{}
	var helmChart *chart.Chart

	flags := genericclioptions.ConfigFlags{
		Namespace: &options.Namespace,
		Context:   &kubeContext,
	}

	helmConf, err := initHelmConfig(&helmOutput, &flags)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Helm config, err: %w, Helm output: %s", err, helmOutput.String())
	}

	if options.ChartPath == "" {
		helmChart, err = helmAction.HelmChartFromContainerRegistry(options.ChartVersion, helmConf, options.ChartRepo, options.ReleaseName)
	} else {
		helmChart, err = helmAction.LoadChart(options.ChartPath)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load Helm chart, err: %w, Helm output: %s", err, helmOutput.String())
	}

	return helmChart, helmConf, nil
}
