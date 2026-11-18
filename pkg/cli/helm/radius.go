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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/authz"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/engine"
	"helm.sh/helm/v4/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
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

// radiusAuthTarget describes the selected chart's component authorization prerequisites.
// Never pass coalesced defaults as user overrides to a Helm action.
type radiusAuthTarget struct {
	chartAuthSettings
	workloadRequirements
}

type chartAuthSettings struct {
	mode     authz.Mode
	provider authz.TLSProvider
	spiffe   spiffeSettings
}

func resolveRadiusAuthTarget(ctx context.Context, selected *chart.Chart, namespace, release string, current, stored map[string]any, reset bool) (radiusAuthTarget, error) {
	values, settings, err := resolveEffectiveChartValues(selected, namespace, release, current, stored, reset)
	if err != nil {
		return radiusAuthTarget{}, err
	}
	req, err := renderedWorkloadRequirements(ctx, selected, values, namespace, release)
	if err != nil {
		return radiusAuthTarget{}, err
	}
	if req.ManagedCSIVolumes > 0 && (settings.mode == authz.ModeOff || settings.provider != authz.TLSProviderRadius || settings.spiffe.CSIDriverName != req.CSIDriverName) {
		return radiusAuthTarget{}, fmt.Errorf("rendered workload requires managed CSI driver %q, inconsistent with the selected authorization settings", req.CSIDriverName)
	}
	return radiusAuthTarget{settings, req}, nil
}

func resolveEffectiveChartValues(selected *chart.Chart, namespace, release string, current, stored map[string]any, reset bool) (map[string]any, chartAuthSettings, error) {
	if selected == nil || selected.Metadata == nil {
		return nil, chartAuthSettings{}, fmt.Errorf("missing target Radius chart or metadata")
	}
	if reset {
		stored = nil
	}
	values, err := valuesutil.CoalesceValues(selected, helmValues(current, stored))
	if err != nil {
		return nil, chartAuthSettings{}, err
	}
	settings, err := selectedAuthSettings(selected, values, namespace, release)
	return values, settings, err
}

func selectedAuthSettings(selected *chart.Chart, values map[string]any, namespace, release string) (chartAuthSettings, error) {
	var target chartAuthSettings
	if nestedValue(selected.Values, "global", "rbac", "tls", "provider") != nil {
		return renderRadiusAuthTarget(selected, values, namespace, release)
	}
	// Legacy charts have no provider switch. Helm accepts unknown overrides;
	// do not silently treat an ignored setting as a supported configuration.
	rbac, _ := nestedValue(values, "global", "rbac").(map[string]any)
	var err error
	target.mode, err = authzModeFromRBAC(rbac)
	if err != nil {
		return target, err
	}
	for _, key := range []string{"mtls", "tlsProvider"} {
		if _, ok := rbac[key]; ok {
			return target, fmt.Errorf("global.rbac.%s is not supported; use global.rbac.tls.provider", key)
		}
	}
	target.provider = authz.TLSProviderRadius
	if _, ok := rbac["tls"]; ok {
		return target, fmt.Errorf("selected Radius chart does not support global.rbac.tls.provider")
	}
	for _, legacy := range []string{"ucpMTLS", "externalIdentity"} {
		if isTrue(nestedValue(values, "de", legacy, "enabled")) {
			return target, fmt.Errorf("de.%s.enabled is retired; use de.internalAuth.enabled", legacy)
		}
	}
	if nestedValue(values, "de", "internalAuth") != nil {
		return target, fmt.Errorf("selected Radius chart does not support de.internalAuth.enabled")
	}
	target.spiffe = resolveSPIFFESettings(nil, values)
	return target, nil
}

// Evaluate the same helpers as the workloads without rendering Secrets, executing hooks,
// or looking up cluster objects. The gate helper validates the engine selection too.
func renderRadiusAuthTarget(selected *chart.Chart, values map[string]any, namespace, release string) (chartAuthSettings, error) {
	var target chartAuthSettings
	c := &chart.Chart{Metadata: selected.Metadata}
	for _, file := range selected.Templates {
		if filepath.Ext(file.Name) == ".tpl" {
			c.Templates = append(c.Templates, file)
		}
	}
	const name = "templates/auth-target.json"
	const probe = `{{- $_ := include "radius.de.internalAuth.enabled" . -}}
{{ dict "Mode" (include "radius.authz.mode" .) "Provider" (include "radius.authz.tls.provider" .) "TrustDomain" (include "radius.authz.spiffe.trustDomain" .) "CSIDriverName" (include "radius.authz.spiffe.csiDriverName" .) | toJson }}`
	c.Templates = append(c.Templates, &common.File{Name: name, Data: []byte(probe)})
	result, err := engine.Render(c, workloadRenderContext(values, namespace, release))
	if err != nil {
		return target, fmt.Errorf("invalid selected Radius chart authorization settings: %w", err)
	}
	var rendered struct {
		Mode     authz.Mode
		Provider authz.TLSProvider
		spiffeSettings
	}
	if err := json.Unmarshal([]byte(result[c.Name()+"/"+name]), &rendered); err != nil {
		return target, err
	}
	switch rendered.Mode {
	case authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce:
	default:
		return target, fmt.Errorf("unsupported selected Radius chart authorization mode %q", rendered.Mode)
	}
	if rendered.Provider != authz.TLSProviderRadius && rendered.Provider != authz.TLSProviderExternal {
		return target, fmt.Errorf("unsupported selected Radius chart TLS provider %q", rendered.Provider)
	}
	return chartAuthSettings{rendered.Mode, rendered.Provider, rendered.spiffeSettings}, nil
}

type workloadRequirements struct {
	ManagedCSIVolumes int
	CSIDriverName     string
}

func workloadRenderContext(values map[string]any, namespace, release string) common.Values {
	caps := common.DefaultCapabilities.Copy()
	caps.APIVersions = append(slices.Clone(caps.APIVersions), "admissionregistration.k8s.io/v1/ValidatingAdmissionPolicy")
	return common.Values{
		"Values": copyValues(values), "Capabilities": caps,
		"Release": map[string]any{"Namespace": namespace, "Name": release, "Service": "Helm", "Revision": 1, "IsInstall": true, "IsUpgrade": false},
	}
}

// renderedWorkloadRequirements inspects pod templates, not a chart's declaration of
// provider support. This offline render cannot verify connected issuer policy;
// callers preparing raw values should use resolveEffectiveChartValues instead.
func renderedWorkloadRequirements(ctx context.Context, selected *chart.Chart, values map[string]any, namespace, release string) (workloadRequirements, error) {
	var req workloadRequirements
	rendered, err := (engine.Engine{}).RenderWithContext(ctx, selected, workloadRenderContext(values, namespace, release))
	if err != nil {
		return req, fmt.Errorf("render Radius workloads: %w", err)
	}
	configuredDriver, _ := nestedValue(values, "global", "rbac", "spiffe", "csiDriverName").(string)
	for name, manifest := range rendered {
		if filepath.Base(name) == "NOTES.txt" {
			continue
		}
		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
		for {
			var obj struct {
				Kind string
				Spec struct{ Template corev1.PodTemplateSpec }
			}
			err := decoder.Decode(&obj)
			if err == io.EOF {
				break
			}
			if err != nil {
				return req, fmt.Errorf("inspect %s: %w", name, err)
			}
			switch obj.Kind {
			case "Deployment", "DaemonSet", "StatefulSet", "Job", "ReplicaSet":
			default:
				continue
			}
			for _, vol := range obj.Spec.Template.Spec.Volumes {
				if vol.CSI == nil {
					continue
				}
				// Recognize the managed driver under any volume name. Keep the
				// historical spiffe slot visible even when its driver is inconsistent.
				if vol.Name != "spiffe" && vol.CSI.Driver != SPIFFECSIDriverNameDefault && (configuredDriver == "" || vol.CSI.Driver != configuredDriver) {
					continue
				}
				if req.ManagedCSIVolumes > 0 && req.CSIDriverName != vol.CSI.Driver {
					return req, fmt.Errorf("rendered SPIFFE volumes require different CSI drivers")
				}
				req.ManagedCSIVolumes++
				req.CSIDriverName = vol.CSI.Driver
			}
		}
	}
	return req, nil
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
