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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/radius-project/radius/pkg/authz"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/engine"
	"helm.sh/helm/v4/pkg/strvals"
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

// radiusActionMode checks the selected chart before changing any dependencies. The caller
// supplies stored values only for actions that reuse them; coalesced defaults never go to Helm.
func radiusActionMode(selected *chart.Chart, current, stored map[string]any, options ChartOptions, upgrade bool) (authz.Mode, error) {
	settings, err := radiusActionSettings(selected, current, stored, options, upgrade)
	return settings.mode, err
}

type chartAuthSettings struct {
	mode     authz.Mode
	provider authz.TLSProvider
}

func radiusActionSettings(selected *chart.Chart, current, stored map[string]any, options ChartOptions, upgrade bool) (chartAuthSettings, error) {
	if selected == nil {
		return chartAuthSettings{}, fmt.Errorf("missing target Radius chart")
	}
	values := resolveEffectiveChartValues(selected.Values, stored, current)
	if nestedValue(selected.Values, "global", "rbac", "tls", "provider") == nil {
		for _, key := range []string{"tls", "tlsProvider", "mtls"} {
			if nestedValue(values, "global", "rbac", key) != nil {
				return chartAuthSettings{}, fmt.Errorf("selected Radius chart does not support global.rbac.%s", key)
			}
		}
		if nestedValue(values, "de", "internalAuth") != nil {
			return chartAuthSettings{}, fmt.Errorf("selected Radius chart does not support de.internalAuth.enabled")
		}
		for _, key := range []string{"ucpMTLS", "externalIdentity"} {
			if isTrue(nestedValue(values, "de", key, "enabled")) {
				return chartAuthSettings{}, fmt.Errorf("de.%s.enabled is retired; use de.internalAuth.enabled", key)
			}
		}
		mode, err := resolveAuthzMode(selected.Values, stored, current)
		return chartAuthSettings{mode, authz.TLSProviderRadius}, err
	}
	// Use the chart's own mode and engine gates without running hooks, lookups or Secret templates.
	c := &chart.Chart{Metadata: selected.Metadata}
	for _, file := range selected.Templates {
		if filepath.Ext(file.Name) == ".tpl" || file.Name == "templates/global/validate-metrics-port.yaml" {
			c.Templates = append(c.Templates, file)
		}
	}
	const name = "templates/action-mode"
	c.Templates = append(c.Templates, &common.File{Name: name, Data: []byte(`{{- $_ := include "radius.de.internalAuth.enabled" . -}}{{ include "radius.authz.mode" . }}/{{ include "radius.authz.tls.provider" . }}`)})
	result, err := engine.Render(c, common.Values{"Values": values, "Capabilities": common.DefaultCapabilities, "Release": map[string]any{"Name": options.ReleaseName, "Namespace": options.Namespace, "IsUpgrade": upgrade, "IsInstall": !upgrade}})
	if err != nil {
		return chartAuthSettings{}, fmt.Errorf("resolve selected Radius chart authorization: %w", err)
	}
	rawMode, rawProvider, _ := strings.Cut(result[c.Name()+"/"+name], "/")
	provider := authz.TLSProvider(rawProvider)
	if provider != authz.TLSProviderRadius && provider != authz.TLSProviderExternal {
		return chartAuthSettings{}, fmt.Errorf("unsupported Radius TLS provider %q", provider)
	}
	mode := authz.Mode(rawMode)
	switch mode {
	case authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce:
		return chartAuthSettings{mode, provider}, nil
	default:
		return chartAuthSettings{}, fmt.Errorf("unsupported Radius authorization mode %q", mode)
	}
}

// prepareRadiusChart prepares the Helm chart for Radius and returns the user-supplied
// values parsed from the CLI --set / --set-file flags. The returned values map is
// intentionally separate from helmChart.Values so that Helm's release storage records
// only the user overrides (and so that upgrades can correctly merge with previously
// stored user values via ResetThenReuseValues).
func prepareRadiusChart(ctx context.Context, helmAction HelmAction, options RadiusChartOptions, kubeContext string) (*chart.Chart, *helm.Configuration, map[string]any, error) {
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

	userValues, err := parseUserValuesFromCLI(&options)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse Radius values, err: %w", err)
	}

	return helmChart, helmConf, userValues, nil
}

// parseUserValuesFromCLI parses the --set and --set-file arguments in order (last one wins)
// and returns a fresh map[string]any containing only the user-supplied overrides. It does
// NOT mutate any chart values. This map is intended to be passed as the `vals` argument
// to Helm install/upgrade so that Helm records it as the user-supplied value tree on
// the release.
func parseUserValuesFromCLI(options *RadiusChartOptions) (map[string]any, error) {
	values := map[string]any{}
	if err := parseSetValuesInto(values, options.SetArgs, options.SetFileArgs); err != nil {
		return nil, err
	}

	return values, nil
}

// parseSetValuesInto parses --set style arguments and then --set-file style arguments into values, in order, so
// that the last one wins. Like Helm's --set, each argument overrides any value already in the map.
func parseSetValuesInto(values map[string]any, setArgs []string, setFileArgs []string) error {
	for _, arg := range setArgs {
		if err := strvals.ParseInto(arg, values); err != nil {
			return err
		}
	}

	for _, arg := range setFileArgs {
		reader := func(rs []rune) (any, error) {
			data, err := os.ReadFile(string(rs))
			return string(data), err
		}

		if err := strvals.ParseIntoFile(setFileArg(arg), values, reader); err != nil {
			return err
		}
	}

	return nil
}

// setFileArg returns a --set-file style argument as strvals parses it. strvals reads a backslash as an escape
// character, so the argument uses forward slashes in Windows paths.
func setFileArg(arg string) string {
	if runtime.GOOS == "windows" {
		return filepath.ToSlash(arg)
	}

	return arg
}
