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
	"testing"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"go.yaml.in/yaml/v3"
)

func Test_ParseMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Mode
		wantErr bool
	}{
		{name: "empty defaults to off", input: "", want: ModeOff},
		{name: "off", input: "off", want: ModeOff},
		{name: "dryRun", input: "dryRun", want: ModeDryRun},
		{name: "enforce", input: "enforce", want: ModeEnforce},
		{name: "case sensitive dryRun", input: "dryrun", wantErr: true},
		{name: "case sensitive enforce", input: "Enforce", wantErr: true},
		{name: "case sensitive off", input: "OFF", wantErr: true},
		{name: "unknown", input: "audit", wantErr: true},
		{name: "whitespace", input: " off", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMode(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.input)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_Options_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Mode
		wantErr bool
	}{
		{name: "missing mode defaults to off", input: "{}", want: ModeOff},
		{name: "empty mode defaults to off", input: `mode: ""`, want: ModeOff},
		{name: "off", input: "mode: off", want: ModeOff},
		{name: "dryRun", input: "mode: dryRun", want: ModeDryRun},
		{name: "enforce", input: "mode: enforce", want: ModeEnforce},
		{name: "invalid", input: "mode: audit", wantErr: true},
		{name: "not a string", input: "mode: [a]", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := Options{}
			err := yaml.Unmarshal([]byte(tt.input), &opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, opts.EffectiveMode())
		})
	}
}

func Test_Options_EffectiveCADirectory(t *testing.T) {
	t.Parallel()

	var opts Options
	require.NoError(t, yaml.Unmarshal([]byte("certificateDirectory: /var/run/radius/spiffe\ncaDirectory: /var/run/radius/ca"), &opts))
	require.Equal(t, "/var/run/radius/ca", opts.EffectiveCADirectory())

	opts.CADirectory = ""
	require.Equal(t, "/var/run/radius/spiffe", opts.EffectiveCADirectory(), "falls back to certificateDirectory")
}

func Test_Options_TLSProvider(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input   string
		want    TLSProvider
		invalid bool
	}{
		{input: "{}", want: TLSProviderRadius},
		{input: "tlsProvider: radius", want: TLSProviderRadius},
		{input: "tlsProvider: external", want: TLSProviderExternal},
		{input: "tlsProvider: typo", invalid: true},
		{input: "tlsProvider: ''", invalid: true},
		{input: "tlsProvider: false", invalid: true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var options Options
			err := yaml.Unmarshal([]byte(tc.input), &options)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, options.ValidateTLSProvider())
			require.Equal(t, tc.want, options.EffectiveTLSProvider())
		})
	}
	require.ErrorContains(t, (Options{TLSProvider: "typo"}).ValidateTLSProvider(), "invalid TLS provider")
}

func Test_LogMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode Mode
		want string
	}{
		{name: "unset", mode: "", want: "authz mode=off"},
		{name: "off", mode: ModeOff, want: "authz mode=off"},
		{name: "dryRun", mode: ModeDryRun, want: "authz mode=dryRun"},
		{name: "enforce", mode: ModeEnforce, want: "authz mode=enforce"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			core, logs := observer.New(zap.InfoLevel)
			LogMode(zapr.NewLogger(zap.New(core)), Options{Mode: tt.mode})

			entries := logs.All()
			require.Len(t, entries, 1)
			require.Equal(t, tt.want, entries[0].Message)
		})
	}
}
