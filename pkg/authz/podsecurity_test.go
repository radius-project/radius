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

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func Test_ParsePodSecurityLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		want    PodSecurityLevel
		wantErr bool
	}{
		{input: "", want: PodSecurityLevelBaseline},
		{input: "baseline", want: PodSecurityLevelBaseline},
		{input: "restricted", want: PodSecurityLevelRestricted},
		{input: "privileged", wantErr: true},
		{input: "Baseline", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParsePodSecurityLevel(tt.input)
			if tt.wantErr {
				require.ErrorContains(t, err, "invalid pod security level")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_Options_UnmarshalPodSecurityLevel(t *testing.T) {
	t.Parallel()

	var opts Options
	require.NoError(t, yaml.Unmarshal([]byte("mode: enforce\npodSecurityLevel: restricted\ncontrolPlaneNamespace: radius\n"), &opts))
	require.Equal(t, Options{Mode: ModeEnforce, PodSecurityLevel: PodSecurityLevelRestricted, ControlPlaneNamespace: "radius"}, opts)

	require.ErrorContains(t, yaml.Unmarshal([]byte("podSecurityLevel: privileged\n"), &Options{}), "invalid pod security level")
}

func Test_Options_PodSecurityLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      Options
		namespace string
		want      map[string]string
	}{
		{name: "unset mode", opts: Options{}, namespace: "apps", want: nil},
		{name: "off", opts: Options{Mode: ModeOff, PodSecurityLevel: PodSecurityLevelRestricted}, namespace: "apps", want: nil},
		{
			name:      "dryRun baseline default",
			opts:      Options{Mode: ModeDryRun},
			namespace: "apps",
			want:      map[string]string{PodSecurityLabelWarn: "baseline", PodSecurityLabelAudit: "baseline"},
		},
		{
			name:      "dryRun restricted",
			opts:      Options{Mode: ModeDryRun, PodSecurityLevel: PodSecurityLevelRestricted},
			namespace: "apps",
			want:      map[string]string{PodSecurityLabelWarn: "restricted", PodSecurityLabelAudit: "restricted"},
		},
		{
			name:      "enforce baseline",
			opts:      Options{Mode: ModeEnforce, PodSecurityLevel: PodSecurityLevelBaseline},
			namespace: "apps",
			want:      map[string]string{PodSecurityLabelEnforce: "baseline", PodSecurityLabelWarn: "baseline", PodSecurityLabelAudit: "baseline"},
		},
		{
			name:      "enforce restricted",
			opts:      Options{Mode: ModeEnforce, PodSecurityLevel: PodSecurityLevelRestricted},
			namespace: "apps",
			want:      map[string]string{PodSecurityLabelEnforce: "restricted", PodSecurityLabelWarn: "restricted", PodSecurityLabelAudit: "restricted"},
		},
		{name: "default control-plane namespace", opts: Options{Mode: ModeEnforce}, namespace: "radius-system", want: nil},
		{name: "configured control-plane namespace", opts: Options{Mode: ModeEnforce, ControlPlaneNamespace: "radius"}, namespace: "radius", want: nil},
		{name: "kube-system", opts: Options{Mode: ModeDryRun}, namespace: "kube-system", want: nil},
		{name: "kube-public", opts: Options{Mode: ModeDryRun}, namespace: "kube-public", want: nil},
		{name: "kube-node-lease", opts: Options{Mode: ModeDryRun}, namespace: "kube-node-lease", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.opts.PodSecurityLabels(tt.namespace))
		})
	}
}

func Test_MissingLabels(t *testing.T) {
	t.Parallel()

	want := map[string]string{PodSecurityLabelWarn: "baseline", PodSecurityLabelAudit: "baseline"}

	tests := []struct {
		name     string
		existing map[string]string
		expected map[string]string
	}{
		{name: "no labels", existing: nil, expected: want},
		{name: "unrelated labels", existing: map[string]string{"team": "a"}, expected: want},
		{name: "operator set warn", existing: map[string]string{PodSecurityLabelWarn: "privileged"}, expected: map[string]string{PodSecurityLabelAudit: "baseline"}},
		{name: "all set", existing: map[string]string{PodSecurityLabelWarn: "restricted", PodSecurityLabelAudit: "privileged"}, expected: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, MissingLabels(tt.existing, want))
		})
	}
}
