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
	"fmt"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/spf13/pflag"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
)

// MTLSProviderOptions selects internal transport identity independently of certificate mode.
type MTLSProviderOptions struct {
	Provider string
}

// AddFlags registers the provider selector without overwriting omitted upgrade values.
func (o *MTLSProviderOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.Provider, "mtls-provider", "", "Internal encryption provider: radius (default) or external (operator encrypts every connection, including storage and queues)")
}

// ValidateFlags rejects explicit empty selectors as well as unknown providers.
func (o MTLSProviderOptions) ValidateFlags(flags *pflag.FlagSet) error {
	if flags != nil && flags.Changed("mtls-provider") && o.Provider == "" {
		return fmt.Errorf("--mtls-provider must not be empty")
	}
	return (authz.Options{MTLSProvider: o.Provider}).ValidateProvider()
}

func providerValue(values map[string]any) (string, bool, error) {
	global, _ := values["global"].(map[string]any)
	rbac, _ := global["rbac"].(map[string]any)
	raw, exists := rbac["mtls"]
	if !exists {
		return "radius", false, nil
	}
	mtls, ok := raw.(map[string]any)
	if !ok {
		return "", true, fmt.Errorf("global.rbac.mtls must be a mapping")
	}
	raw, exists = mtls["provider"]
	if !exists {
		return "radius", false, nil
	}
	provider, ok := raw.(string)
	if !ok || (provider != "radius" && provider != "external") {
		return "", true, fmt.Errorf("global.rbac.mtls.provider must be radius or external")
	}
	return provider, true, nil
}

func (i *Impl) prepareMTLSProvider(options ClusterOptions, defaults, current map[string]any, installed bool) error {
	if err := options.MTLS.ValidateFlags(nil); err != nil {
		return err
	}
	old, storedSelection, err := providerValue(options.RadiusReleaseValues)
	if err != nil {
		return err
	}
	_, currentSelection, err := providerValue(current)
	if err != nil {
		return err
	}
	if _, _, err := providerValue(defaults); err != nil {
		return err
	}
	if options.MTLS.Provider != "" && currentSelection {
		return fmt.Errorf("--mtls-provider cannot be combined with global.rbac.mtls.provider overrides")
	}
	if options.ResetValues && storedSelection {
		return fmt.Errorf("--reset-values cannot discard stored mTLS provider settings")
	}
	merged := valuesutil.CoalesceTables(copyValues(current), copyValues(options.RadiusReleaseValues))
	merged = valuesutil.CoalesceTables(merged, copyValues(defaults))
	provider, _, err := providerValue(merged)
	if err != nil {
		return err
	}
	if options.MTLS.Provider != "" {
		provider = options.MTLS.Provider
	}
	if installed && old != provider {
		return fmt.Errorf("mTLS provider changes require a coordinated restart of all callers and receivers; keep the installed provider")
	}
	mode, err := resolveAuthzMode(nil, merged)
	if err != nil {
		return err
	}
	if mode == authz.ModeOff && (options.MTLS.Provider != "" || (currentSelection && provider == "external")) {
		return fmt.Errorf("--mtls-provider requires global.rbac.dryRun=true or global.rbac.enabled=true")
	}
	de, _ := merged["de"].(map[string]any)
	external, _ := de["externalIdentity"].(map[string]any)
	enabled := false
	if raw, exists := external["enabled"]; exists {
		var ok bool
		enabled, ok = raw.(bool)
		if !ok {
			return fmt.Errorf("de.externalIdentity.enabled must be a boolean")
		}
	}
	if enabled && provider != "external" {
		return fmt.Errorf("de.externalIdentity.enabled requires global.rbac.mtls.provider=external")
	}
	if provider == "external" {
		if certificateMode(merged) != "selfSigned" || options.Certificates.Specified() {
			return fmt.Errorf("external provider supports only the existing selfSigned aggregation certificate; caSecret/issuerRef require coordinated aggregation CA migration")
		}
		image, _ := de["image"].(string)
		tag, _ := de["tag"].(string)
		if mode != authz.ModeOff && (!enabled || image == "" || tag == "") {
			return fmt.Errorf("external provider requires de.externalIdentity.enabled=true and an explicit de.image/de.tag for a paired token-capable engine; the published default image is not supported")
		}
		mtls, _ := de["ucpMTLS"].(map[string]any)
		proof, _ := rbacSettings(merged)["startupProof"].(map[string]any)
		if isTrue(mtls["enabled"]) || isTrue(proof["enabled"]) {
			return fmt.Errorf("external provider cannot use component mTLS or TLS startup proof")
		}
		i.logInfo("Warning: external mTLS relies on operator-provided encryption and integrity for EVERY connection, including storage and queues. Radius cannot verify this protection.")
	}
	if options.MTLS.Provider != "" {
		global, ok := current["global"].(map[string]any)
		if !ok {
			global = map[string]any{}
			current["global"] = global
		}
		rbac, ok := global["rbac"].(map[string]any)
		if !ok {
			rbac = map[string]any{}
			global["rbac"] = rbac
		}
		rbac["mtls"] = map[string]any{"provider": provider}
	}
	return nil
}

func rbacSettings(values map[string]any) map[string]any {
	global, _ := values["global"].(map[string]any)
	rbac, _ := global["rbac"].(map[string]any)
	return rbac
}
