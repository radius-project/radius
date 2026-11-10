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
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
)

func externalProviderValues() map[string]any {
	return map[string]any{
		"global": map[string]any{"rbac": map[string]any{
			"dryRun": true, "mtls": map[string]any{"provider": "external"},
		}},
		"de": map[string]any{"image": "local-engine", "tag": "local-test", "externalIdentity": map[string]any{"enabled": true}},
	}
}

func TestMTLSProviderFlags(t *testing.T) {
	for _, value := range []string{"", "radius", "external", "invalid"} {
		t.Run(value, func(t *testing.T) {
			var o MTLSProviderOptions
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			o.AddFlags(flags)
			require.NoError(t, o.ValidateFlags(flags))
			require.NoError(t, flags.Set("mtls-provider", value))
			err := o.ValidateFlags(flags)
			if value == "" || value == "invalid" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPrepareMTLSProvider(t *testing.T) {
	for _, tt := range []struct {
		name      string
		flag      string
		installed bool
		reset     bool
		stored    map[string]any
		values    map[string]any
		mutate    func(map[string]any)
		wantErr   string
	}{
		{name: "default Off unchanged", values: map[string]any{}},
		{name: "external fresh", values: externalProviderValues()},
		{name: "omitted upgrade preserves external", installed: true, stored: externalProviderValues(), values: map[string]any{}},
		{name: "Off upgrade preserves dormant external provider", installed: true, stored: externalProviderValues(), values: map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": false}}}},
		{name: "explicit same upgrade", flag: "external", installed: true, stored: externalProviderValues(), values: map[string]any{}},
		{name: "unknown flag", flag: "mesh", values: map[string]any{}, wantErr: "invalid mTLS"},
		{name: "empty value", values: externalProviderValues(), mutate: func(v map[string]any) { rbacSettings(v)["mtls"] = map[string]any{"provider": ""} }, wantErr: "must be radius"},
		{name: "invalid shape", values: externalProviderValues(), mutate: func(v map[string]any) { rbacSettings(v)["mtls"] = "external" }, wantErr: "mapping"},
		{name: "flag overrides conflict", flag: "external", values: externalProviderValues(), wantErr: "cannot be combined"},
		{name: "Off external rejected", values: externalProviderValues(), mutate: func(v map[string]any) { rbacSettings(v)["dryRun"] = false }, wantErr: "requires global.rbac"},
		{name: "explicit radius Off rejected", flag: "radius", values: map[string]any{}, wantErr: "requires global.rbac"},
		{name: "provider transition rejected", installed: true, values: externalProviderValues(), wantErr: "coordinated restart"},
		{name: "reverse transition rejected", installed: true, stored: externalProviderValues(), flag: "radius", values: map[string]any{}, wantErr: "coordinated restart"},
		{name: "reset rejected", installed: true, reset: true, stored: externalProviderValues(), values: map[string]any{}, wantErr: "reset-values"},
		{name: "independent CA Secret selection", values: externalProviderValues(), mutate: func(v map[string]any) { setCertificateValues(v, map[string]any{"mode": "caSecret"}) }},
		{name: "independent external issuer selection", values: externalProviderValues(), mutate: func(v map[string]any) { setCertificateValues(v, map[string]any{"mode": "issuerRef"}) }},
		{name: "unpaired engine rejected", values: externalProviderValues(), mutate: func(v map[string]any) { delete(v, "de") }, wantErr: "paired token-capable"},
		{name: "implicit engine tag rejected", values: externalProviderValues(), mutate: func(v map[string]any) { delete(v["de"].(map[string]any), "tag") }, wantErr: "paired token-capable"},
		{name: "engine opt-in must be boolean", values: externalProviderValues(), mutate: func(v map[string]any) {
			v["de"].(map[string]any)["externalIdentity"] = map[string]any{"enabled": "true"}
		}, wantErr: "must be a boolean"},
		{name: "engine opt-in requires external", values: externalProviderValues(), mutate: func(v map[string]any) { rbacSettings(v)["mtls"] = map[string]any{"provider": "radius"} }, wantErr: "requires global.rbac.mtls.provider"},
		{name: "engine TLS conflict", values: externalProviderValues(), mutate: func(v map[string]any) { v["de"].(map[string]any)["ucpMTLS"] = map[string]any{"enabled": true} }, wantErr: "cannot use component mTLS"},
		{name: "proof conflict", values: externalProviderValues(), mutate: func(v map[string]any) { rbacSettings(v)["startupProof"] = map[string]any{"enabled": true} }, wantErr: "cannot use component mTLS"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			options := NewDefaultClusterOptions()
			options.MTLS.Provider, options.RadiusReleaseValues, options.ResetValues = tt.flag, tt.stored, tt.reset
			if tt.mutate != nil {
				tt.mutate(tt.values)
			}
			var warnings []string
			impl := &Impl{logger: func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }}
			err := impl.prepareMTLSProvider(options, nil, tt.values, tt.installed)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if len(warnings) > 0 {
				require.Contains(t, warnings[0], "EVERY connection, including storage and queues")
			}
			if tt.flag != "" {
				provider, _, err := providerValue(tt.values)
				require.NoError(t, err)
				require.Equal(t, tt.flag, provider)
			}
		})
	}
}

func TestMTLSProviderOffDoesNotProvisionCertManager(t *testing.T) {
	options := NewDefaultClusterOptions()
	options.Radius.SetArgs = []string{"global.rbac.enabled=false", "global.rbac.mtls.provider=external"}
	// No Helm/Kubernetes clients: accessing either would panic.
	require.NoError(t, (&Impl{}).ensureCertManager(t.Context(), options, "", nil, false))
}

func TestExternalProviderRejectsRotationWhileOff(t *testing.T) {
	for _, phase := range []string{"publish", "adopt", "issue", "retire", "recover"} {
		for _, readOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/preflight=%t", phase, readOnly), func(t *testing.T) {
				options := NewDefaultClusterOptions()
				options.Certificates.RotationPhase = phase
				values := externalProviderValues()
				rbacSettings(values)["dryRun"] = false
				client := NewMockHelmClient(gomock.NewController(t))
				client.EXPECT().RunHelmGet(gomock.Any(), radiusReleaseName).Return(&releasev1.Release{
					Chart:  &chart.Chart{Metadata: &chart.Metadata{Annotations: map[string]string{"radius.io/ca-rotation-protocol": "1"}}},
					Config: values,
					Info:   &releasev1.Info{Status: releasecommon.StatusDeployed},
				}, nil)
				impl := &Impl{Helm: client}
				var err error
				if readOnly {
					err = impl.CheckRadiusCertificates(t.Context(), options, "")
				} else {
					err = impl.UpgradeRadius(t.Context(), options, "")
				}
				require.ErrorContains(t, err, "enabling authorization is a separate upgrade")
			})
		}
	}
}

func TestRotationRejectsProviderSelectorBeforeClusterAccess(t *testing.T) {
	for _, provider := range []string{"radius", "external"} {
		options := NewDefaultClusterOptions()
		options.MTLS.Provider = provider
		options.Certificates.RotationPhase = "publish"
		require.ErrorContains(t, (&Impl{}).runCertificateRotation(t.Context(), options, "", false), "cannot be combined")
	}
}
