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

package hostoptions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
)

func Test_loadConfig_ControllerDevelopmentDefaults(t *testing.T) {
	t.Parallel()
	config, err := loadConfig(filepath.Join("..", "..", "..", "cmd", "controller", "controller-dev.yaml"))
	require.NoError(t, err)
	require.False(t, config.Controller.Flux.Enabled)
	require.Empty(t, config.Controller.Flux.Sources)
	require.NotNil(t, config.WorkerServer)
	require.NotNil(t, config.WorkerServer.Port)
}

func Test_loadConfig_AuthorizationMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    authz.Mode
		wantErr bool
	}{
		{name: "missing defaults to off", input: "logging:\n  level: info\n", want: authz.ModeOff},
		{name: "off", input: "authorization:\n  mode: off\n", want: authz.ModeOff},
		{name: "dryRun", input: "authorization:\n  mode: dryRun\n", want: authz.ModeDryRun},
		{name: "enforce", input: "authorization:\n  mode: enforce\n", want: authz.ModeEnforce},
		{name: "invalid", input: "authorization:\n  mode: audit\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tt.input), 0o600))

			config, err := loadConfig(path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, config.Authorization.EffectiveMode())
		})
	}
}

func Test_loadConfig_FluxSources(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		enabled bool
		count   int
		wantErr string
	}{
		{name: "omitted", input: "logging:\n  level: info\n"},
		{name: "disabled", input: "controller:\n  flux:\n    enabled: false\n    sources: []\n"},
		{name: "empty enabled", input: "controller:\n  flux:\n    enabled: true\n    sources: []\n", enabled: true},
		{name: "selected", input: "controller:\n  flux:\n    enabled: true\n    sources:\n      - namespace: flux-system\n        name: apps\n        uid: source-uid\n", enabled: true, count: 1},
		{name: "missing uid", input: "controller:\n  flux:\n    sources:\n      - namespace: flux-system\n        name: apps\n", wantErr: ".uid"},
		{name: "duplicate", input: "controller:\n  flux:\n    sources:\n      - {namespace: flux-system, name: apps, uid: first}\n      - {namespace: flux-system, name: apps, uid: second}\n", wantErr: "duplicate"},
		{name: "wildcard namespace", input: "controller:\n  flux:\n    sources:\n      - {namespace: '*', name: apps, uid: first}\n", wantErr: ".namespace"},
		{name: "invalid name", input: "controller:\n  flux:\n    sources:\n      - {namespace: flux-system, name: 'a/b', uid: first}\n", wantErr: ".name"},
		{name: "wildcard uid", input: "controller:\n  flux:\n    sources:\n      - {namespace: flux-system, name: apps, uid: '*'}\n", wantErr: ".uid"},
		{name: "unknown field", input: "controller:\n  flux:\n    selector: '*'\n", wantErr: "field selector"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tt.input), 0o600))
			config, err := loadConfig(path)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.enabled, config.Controller.Flux.Enabled)
			require.Len(t, config.Controller.Flux.Sources, tt.count)
		})
	}
}
