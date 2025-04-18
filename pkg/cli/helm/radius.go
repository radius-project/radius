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
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	radiusReleaseName = "radius"
	radiusHelmRepo    = "oci://ghcr.io/radius-project/helm-chart"
	// RadiusSystemNamespace is the default namespace for Radius.
	RadiusSystemNamespace = "radius-system"
)

type RadiusChartOptions struct {
	ChartOptions
}

// TODO COMMENT
func prepareRadiusChart(helmAction HelmAction, options RadiusChartOptions, kubeContext string) (*chart.Chart, *action.Configuration, error) {
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

	err = addArgsFromCLI(helmChart, &options)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add Radius values, err: %w, Helm output: %s", err, helmOutput.String())
	}

	return helmChart, helmConf, nil
}

// addArgsFromCLI parses the --set arguments in order and adds them to the Helm chart values
func addArgsFromCLI(helmChart *chart.Chart, options *RadiusChartOptions) error {
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

	return nil
}
