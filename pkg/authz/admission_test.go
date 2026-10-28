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

package authz

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"go.yaml.in/yaml/v3"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

func Test_Options_AdmitNamespace(t *testing.T) {
	tests := []struct {
		name          string
		opts          Options
		namespace     string
		wantCode      string
		wantWouldDeny bool
	}{
		{name: "off", opts: Options{}, namespace: "radius-system"},
		{name: "dryRun default namespace", opts: Options{Mode: ModeDryRun}, namespace: "radius-system", wantWouldDeny: true},
		{name: "dryRun other namespace", opts: Options{Mode: ModeDryRun}, namespace: "default"},
		{name: "enforce default namespace", opts: Options{Mode: ModeEnforce}, namespace: "radius-system", wantCode: v1.CodeAdmissionPolicyDenied},
		{name: "enforce case-insensitive", opts: Options{Mode: ModeEnforce}, namespace: "Radius-System", wantCode: v1.CodeAdmissionPolicyDenied},
		{name: "enforce configured namespace", opts: Options{Mode: ModeEnforce, ControlPlaneNamespace: "radius"}, namespace: "radius", wantCode: v1.CodeAdmissionPolicyDenied},
		{name: "enforce default not control plane when configured", opts: Options{Mode: ModeEnforce, ControlPlaneNamespace: "radius"}, namespace: "radius-system"},
		{name: "enforce other namespace", opts: Options{Mode: ModeEnforce}, namespace: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))

			err := tt.opts.AdmitNamespace(ctx, "Applications.Core/environments/write", "/env", tt.namespace)
			if tt.wantCode != "" {
				denied, ok := errors.AsType[*DeniedError](err)
				require.True(t, ok, "expected a denial, got %v", err)
				require.Equal(t, tt.wantCode, denied.Code)
				require.Equal(t, RuleControlPlaneNamespace, denied.Rule)
				require.Equal(t, 1, logs.FilterField(zap.Bool(LogFieldDenied, true)).FilterField(zap.String(LogFieldRule, RuleControlPlaneNamespace)).Len())
				return
			}

			require.NoError(t, err)
			wouldDeny := logs.FilterField(zap.Bool(LogFieldWouldDeny, true)).FilterField(zap.String(LogFieldRule, RuleControlPlaneNamespace))
			require.Equal(t, tt.wantWouldDeny, wouldDeny.Len() == 1)
			if tt.opts.Mode == ModeOff {
				require.Zero(t, logs.Len())
			}
		})
	}
}

func Test_Options_IsControlPlaneSecret(t *testing.T) {
	opts := Options{ControlPlaneSecrets: []string{"ucp-cert", "database-secret"}}
	tests := []struct {
		name   string
		opts   Options
		secret string
		want   bool
	}{
		{name: "listed", opts: opts, secret: "ucp-cert", want: true},
		{name: "not listed", opts: opts, secret: "app-secret"},
		{name: "empty name", opts: opts, secret: ""},
		{name: "no configured secrets", opts: Options{}, secret: "ucp-cert"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.opts.IsControlPlaneSecret(tt.secret))
		})
	}
}

func Test_Options_EffectiveControlPlaneNamespace(t *testing.T) {
	require.Equal(t, "radius-system", Options{}.EffectiveControlPlaneNamespace())
	require.Equal(t, "radius", Options{ControlPlaneNamespace: "radius"}.EffectiveControlPlaneNamespace())
}

func Test_Options_UnmarshalControlPlaneSecrets(t *testing.T) {
	var opts Options
	err := yaml.Unmarshal([]byte("mode: enforce\ncontrolPlaneSecrets: [\"ucp-cert\",\"database-secret\"]\n"), &opts)
	require.NoError(t, err)
	require.Equal(t, []string{"ucp-cert", "database-secret"}, opts.ControlPlaneSecrets)
}
