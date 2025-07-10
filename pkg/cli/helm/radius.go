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

// prepareRadiusChart prepares the Helm chart for Radius.
func prepareRadiusChart(helmAction HelmAction, options RadiusChartOptions, kubeContext string) (*chart.Chart, *action.Configuration, error) {
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
		helmChart, err = helmAction.HelmChartFromContainerRegistry(options.ChartVersion, helmConf, options.ChartRepo, options.ReleaseName)
	} else {
		helmChart, err = helmAction.LoadChart(options.ChartPath)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load Helm chart, err: %w", err)
	}

	err = addArgsFromCLI(helmChart, &options)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add Radius values, err: %w", err)
	}

	return helmChart, helmConf, nil
}

// addArgsFromCLI parses the --set arguments in order and adds them to the Helm chart values
func addArgsFromCLI(helmChart *chart.Chart, options *RadiusChartOptions) error {
	values := helmChart.Values

	// Handle TerraformContainer flag by setting the appropriate Helm values
	if options.TerraformContainer != "" {
		// Parse the container image into image and tag
		image, tag := parseContainerImage(options.TerraformContainer)

		// Enable terraform container feature
		err := strvals.ParseInto("global.terraform.enabled=true", values)
		if err != nil {
			return err
		}

		// Set the container image
		err = strvals.ParseInto(fmt.Sprintf("global.terraform.image=%s", image), values)
		if err != nil {
			return err
		}

		// Set the container tag
		err = strvals.ParseInto(fmt.Sprintf("global.terraform.tag=%s", tag), values)
		if err != nil {
			return err
		}
	}

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

// parseContainerImage parses a container image string into image and tag components.
// Examples:
// - "hashicorp/terraform:latest" -> ("hashicorp/terraform", "latest")
// - "ghcr.io/hashicorp/terraform:1.6.0" -> ("ghcr.io/hashicorp/terraform", "1.6.0")
// - "myregistry.azurecr.io/terraform:latest" -> ("myregistry.azurecr.io/terraform", "latest")
// - "hashicorp/terraform" -> ("hashicorp/terraform", "latest")
func parseContainerImage(containerImage string) (string, string) {
	parts := strings.Split(containerImage, ":")
	if len(parts) == 1 {
		// No tag specified, use latest
		return parts[0], "latest"
	}

	// Handle cases where the registry URL contains a port (e.g., localhost:5000/image:tag)
	if len(parts) > 2 {
		// Join all parts except the last one as the image name
		image := strings.Join(parts[:len(parts)-1], ":")
		tag := parts[len(parts)-1]
		return image, tag
	}

	return parts[0], parts[1]
}
