/*
Copyright 2026 The Radius Authors.

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
	"errors"
	"fmt"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/spf13/pflag"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var errResetDiscardsStoredProvider = errors.New("--reset-values cannot discard stored TLS provider settings")

var errProviderObservationUnavailable = errors.New("TLS provider transition blocked before mutation: production loaded-trust reporting and authorized observation are unavailable for Radius callers (including the trust-only controller), engine UCP trust, UCP downstream authorities and every aggregation cache; receiver readiness also lacks approved CLI credentials; do not mint component tokens, extract credentials, forward API bearer tokens or bypass the gate")

// TLSProviderOptions selects transport independently of caller authentication and certificate mode.
type TLSProviderOptions struct {
	Provider string
	legacy   string
}

// AddFlags registers the selector and an explicit diagnostic for the former flag.
func (o *TLSProviderOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.Provider, "tls-provider", "", "Internal encryption provider: radius (default) or external (operator protects every connection, including storage and queues)")
	flags.StringVar(&o.legacy, "mtls-provider", "", "Unsupported legacy provider selector; use --tls-provider on a token-authenticated installation")
}

// ValidateFlags never aliases legacy certificate-client identity to token identity.
func (o TLSProviderOptions) ValidateFlags(flags *pflag.FlagSet) error {
	if o.legacy != "" || flags != nil && flags.Changed("mtls-provider") {
		return errors.New("--mtls-provider is unsupported; use --tls-provider for token-authenticated installations; legacy installations and journals require a separately supported migration")
	}
	if flags != nil && flags.Changed("tls-provider") && o.Provider == "" {
		return errors.New("--tls-provider must not be empty")
	}
	return (authz.Options{TLSProvider: authz.TLSProvider(o.Provider)}).ValidateTLSProvider()
}

func providerValue(values map[string]any) (string, bool, error) {
	raw, found, err := unstructured.NestedFieldNoCopy(values, "global", "rbac")
	if err != nil {
		return "", false, fmt.Errorf("invalid global.rbac mapping: %w", err)
	}
	if found {
		// Helm historically accepts these values to turn RBAC off. A stored
		// external selection is still protected by the installed-provider guard.
		disabled := raw == nil || raw == false || raw == "" || raw == int64(0) || raw == float64(0) || raw == int(0)
		if disabled {
			return string(authz.TLSProviderRadius), false, nil
		}
		if _, ok := raw.(map[string]any); !ok {
			return "", false, errors.New("invalid global.rbac mapping")
		}
	}
	if _, found, err := unstructured.NestedFieldNoCopy(values, "global", "rbac", "mtls", "provider"); err != nil || found {
		return "", false, errors.New("global.rbac.mtls.provider is unsupported; select global.rbac.tls.provider only after a supported legacy installation migration; existing external settings must not default to radius")
	}
	if _, found, err := unstructured.NestedFieldNoCopy(values, "global", "rbac", "tlsProvider"); err != nil || found {
		return "", false, errors.New("use Helm global.rbac.tls.provider, not global.rbac.tlsProvider")
	}
	for _, name := range []string{"providerGeneration", "providerReplicas"} {
		if _, found, err := unstructured.NestedFieldNoCopy(values, "global", "rbac", name); err != nil || found {
			return "", false, fmt.Errorf("global.rbac.%s belongs to deferred provider execution; preserve saved values for investigation: %w", name, errProviderObservationUnavailable)
		}
	}
	value, found, err := unstructured.NestedString(values, "global", "rbac", "tls", "provider")
	if err != nil {
		return "", false, fmt.Errorf("invalid global.rbac.tls.provider: %w", err)
	}
	if !found {
		return string(authz.TLSProviderRadius), false, nil
	}
	if value != "radius" && value != "external" {
		return "", true, errors.New("global.rbac.tls.provider must be radius or external")
	}
	return value, true, nil
}

// A deployed chart without the provider key ignored a stored override, as A6
// did, so resetting values discards no provider setting that was in effect.
func storedProviderHonored(deployed *chart.Chart) bool {
	if deployed == nil {
		return true
	}
	_, found, err := providerValue(deployed.Values)
	return found || err != nil
}

func (i *Impl) prepareTLSProvider(options ClusterOptions, selected *chart.Chart, current map[string]any, baseline *chartAuthSettings) (map[string]any, chartAuthSettings, error) {
	if err := options.TLS.ValidateFlags(nil); err != nil {
		return nil, chartAuthSettings{}, err
	}
	_, stored, err := providerValue(options.RadiusReleaseValues)
	if err != nil {
		return nil, chartAuthSettings{}, err
	}
	_, explicit, err := providerValue(current)
	if err != nil {
		return nil, chartAuthSettings{}, err
	}
	if selected == nil {
		return nil, chartAuthSettings{}, errors.New("missing target Radius chart")
	}
	if _, _, err := providerValue(selected.Values); err != nil {
		return nil, chartAuthSettings{}, err
	}
	if options.TLS.Provider != "" && explicit {
		return nil, chartAuthSettings{}, errors.New("--tls-provider cannot be combined with global.rbac.tls.provider overrides")
	}
	if options.ResetValues && stored && storedProviderHonored(options.RadiusReleaseChart) {
		return nil, chartAuthSettings{}, errResetDiscardsStoredProvider
	}
	overrides := current
	if options.TLS.Provider != "" {
		overrides = copyValues(current)
		if err := unstructured.SetNestedField(overrides, options.TLS.Provider, "global", "rbac", "tls", "provider"); err != nil {
			return nil, chartAuthSettings{}, err
		}
	}
	saved := options.RadiusReleaseValues
	if options.ResetValues {
		saved = nil
	}
	merged := resolveEffectiveChartValues(selected.Values, saved, overrides)
	target, err := radiusActionSettings(selected, overrides, saved, options.Radius.ChartOptions, options.RadiusInstalled || options.RadiusReleaseValues != nil)
	if err != nil {
		return nil, chartAuthSettings{}, err
	}
	if baseline != nil && baseline.provider != target.provider {
		return nil, chartAuthSettings{}, errProviderObservationUnavailable
	}
	if (explicit || stored && !options.ResetValues || options.TLS.Provider != "") && (selected.Metadata == nil || selected.Metadata.Annotations["radius.io/provider-transition-protocol"] != "2") {
		return nil, chartAuthSettings{}, errors.New("selected chart does not support TLS provider protocol v2")
	}
	if target.mode == authz.ModeOff && (options.TLS.Provider != "" || explicit && target.provider == authz.TLSProviderExternal) {
		return nil, chartAuthSettings{}, errors.New("--tls-provider requires active authorization; Off is not a transition bridge")
	}
	// Retain the strict retired-engine flag diagnostics even for chart helpers that accept falsy scalars.
	for _, name := range []string{"ucpMTLS", "externalIdentity"} {
		enabled, _, err := unstructured.NestedBool(merged, "de", name, "enabled")
		if err != nil || enabled {
			return nil, chartAuthSettings{}, fmt.Errorf("de.%s.enabled is unsupported; a paired token-capable engine uses de.internalAuth.enabled", name)
		}
	}
	if options.TLS.Provider != "" {
		if err := unstructured.SetNestedField(current, options.TLS.Provider, "global", "rbac", "tls", "provider"); err != nil {
			return nil, chartAuthSettings{}, err
		}
	}
	if target.provider == authz.TLSProviderExternal && target.mode != authz.ModeOff {
		i.logInfo("Warning: external TLS requires operator encryption and integrity for EVERY connection, including storage and queues. Radius cannot verify this protection.")
	}
	return merged, target, nil
}

// rollbackProviderMode compares the deployed and requested revisions before any operation lease.
func rollbackProviderMode(history []*releasev1.Release, revision int) (authz.Mode, error) {
	var deployed, target *releasev1.Release
	for _, rel := range history {
		if rel == nil {
			continue
		}
		if revisionStatus(rel) == releasecommon.StatusDeployed && (deployed == nil || rel.Version > deployed.Version) {
			deployed = rel
		}
		if rel.Version == revision {
			target = rel
		}
	}
	if deployed == nil {
		return "", errors.New("cannot establish deployed TLS provider: no deployed Radius revision")
	}
	if target == nil {
		return "", fmt.Errorf("rollback revision %d not found", revision)
	}
	old, err := historicalProviderSettings(deployed)
	if err != nil {
		return "", err
	}
	next, err := historicalProviderSettings(target)
	if err != nil {
		return "", err
	}
	if old.provider != next.provider {
		return "", errProviderObservationUnavailable
	}
	return next.mode, nil
}

func historicalProviderSettings(rel *releasev1.Release) (chartAuthSettings, error) {
	if err := validateProviderRevision(rel); err != nil {
		return chartAuthSettings{}, err
	}
	if !storedProviderHonored(rel.Chart) {
		mode, err := resolveAuthzMode(rel.Chart.Values, nil, rel.Config)
		return chartAuthSettings{mode, authz.TLSProviderRadius}, err
	}
	return radiusActionSettings(rel.Chart, rel.Config, nil, ChartOptions{Namespace: rel.Namespace, ReleaseName: rel.Name}, true)
}

func validateProviderRevision(rel *releasev1.Release) error {
	if rel.Chart == nil {
		return errors.New("cannot establish TLS provider: missing Radius chart")
	}
	if _, _, err := providerValue(rel.Chart.Values); err != nil {
		return err
	}
	_, _, err := providerValue(rel.Config)
	return err
}
