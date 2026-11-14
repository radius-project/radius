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
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var errProviderObservationUnavailable = errors.New("TLS provider transition blocked before mutation: production loaded-trust reporting and authorized observation are unavailable for Radius callers (including the trust-only controller), engine UCP trust, UCP downstream authorities and every aggregation cache; receiver readiness also lacks approved CLI credentials; do not mint component tokens, extract credentials, forward API bearer tokens or bypass the gate")

// TLSProviderOptions selects transport independently of caller authentication and certificate mode.
type TLSProviderOptions struct {
	Provider string
	legacy   string
}

// AddFlags registers the selector and an explicit diagnostic for the former flag.
func (o *TLSProviderOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.Provider, "tls-provider", "", "Internal encryption provider: radius (default) or external (operator protects every connection, including storage and queues)")
	flags.StringVar(&o.legacy, "mtls-provider", "", "Unsupported legacy provider selector; use --tls-provider on a token/CSI installation")
}

// ValidateFlags never aliases legacy certificate-client identity to token identity.
func (o TLSProviderOptions) ValidateFlags(flags *pflag.FlagSet) error {
	if o.legacy != "" || flags != nil && flags.Changed("mtls-provider") {
		return errors.New("--mtls-provider is unsupported; use --tls-provider for token/CSI installations; legacy installations and journals require a separately supported migration")
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

func (i *Impl) prepareTLSProvider(options ClusterOptions, defaults, current map[string]any, installed bool) error {
	if err := options.TLS.ValidateFlags(nil); err != nil {
		return err
	}
	old, stored, err := providerValue(options.RadiusReleaseValues)
	if err != nil {
		return err
	}
	_, explicit, err := providerValue(current)
	if err != nil {
		return err
	}
	if _, _, err := providerValue(defaults); err != nil {
		return err
	}
	if options.TLS.Provider != "" && explicit {
		return errors.New("--tls-provider cannot be combined with global.rbac.tls.provider overrides")
	}
	if options.ResetValues && stored {
		return errors.New("--reset-values cannot discard stored TLS provider settings")
	}
	merged := valuesutil.CoalesceTables(copyValues(current), copyValues(options.RadiusReleaseValues))
	merged = valuesutil.CoalesceTables(merged, copyValues(defaults))
	provider, _, err := providerValue(merged)
	if err != nil {
		return err
	}
	if options.TLS.Provider != "" {
		provider = options.TLS.Provider
	}
	if installed && old != provider {
		return errProviderObservationUnavailable
	}
	mode, err := resolveAuthzMode(nil, merged)
	if err != nil {
		return err
	}
	if mode == authz.ModeOff && (options.TLS.Provider != "" || explicit && provider == "external") {
		return errors.New("--tls-provider requires active authorization; Off is not a transition bridge")
	}
	for _, name := range []string{"ucpMTLS", "externalIdentity"} {
		enabled, _, err := unstructured.NestedBool(merged, "de", name, "enabled")
		if err != nil || enabled {
			return fmt.Errorf("de.%s.enabled is unsupported; a paired token-capable engine uses de.internalAuth.enabled", name)
		}
	}
	if options.TLS.Provider != "" {
		if err := unstructured.SetNestedField(current, provider, "global", "rbac", "tls", "provider"); err != nil {
			return err
		}
	}
	if provider == "external" && mode != authz.ModeOff {
		i.logInfo("Warning: external TLS requires operator encryption and integrity for EVERY connection, including storage and queues. Radius cannot verify this protection.")
	}
	return nil
}
