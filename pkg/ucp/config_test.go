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

package ucp

import (
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
)

func Test_LoadConfig_AuthorizationMode(t *testing.T) {
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

			config, err := LoadConfig([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, config.Authorization.EffectiveMode())
		})
	}
}

func Test_LoadConfig_AWS(t *testing.T) {
	t.Parallel()

	config, err := LoadConfig([]byte("aws:\n  fingerprintKeyFile: /tmp/key\n"))
	require.NoError(t, err)
	require.Equal(t, "/tmp/key", config.AWS.FingerprintKeyFile)

	config, err = LoadConfig([]byte("logging:\n  level: info\n"))
	require.NoError(t, err)
	require.Empty(t, config.AWS.FingerprintKeyFile)

	_, err = LoadConfig([]byte("aws:\n  unknown: true\n"))
	require.Error(t, err)
}

func Test_LoadConfig_CallerAuthentication(t *testing.T) {
	t.Parallel()
	config, err := LoadConfig([]byte(`
callerAuthentication:
  caFile: /etc/radius/requestheader-ca.crt
  allowedNames:
    - front-proxy-client
  kubernetesIssuer: kubernetes:cluster-id
`))
	require.NoError(t, err)
	require.NotNil(t, config.CallerAuthentication)
	require.Equal(t, "/etc/radius/requestheader-ca.crt", config.CallerAuthentication.CAFile)
	require.Equal(t, []string{"front-proxy-client"}, config.CallerAuthentication.AllowedNames)
	require.Equal(t, "kubernetes:cluster-id", config.CallerAuthentication.KubernetesIssuer)

	config, err = LoadConfig([]byte("logging:\n  level: info\n"))
	require.NoError(t, err)
	require.Nil(t, config.CallerAuthentication)

	_, err = LoadConfig([]byte("callerAuthentication:\n  allowUnverifiedHeaders: true\n"))
	require.ErrorContains(t, err, "field allowUnverifiedHeaders not found")
}

func Test_LoadConfig_KubernetesCallerAuthentication(t *testing.T) {
	t.Parallel()
	config, err := LoadConfig([]byte("kubernetesCallerAuthentication:\n  kubernetesIssuer: kubernetes:cluster-a\n"))
	require.NoError(t, err)
	require.Equal(t, "kubernetes:cluster-a", config.KubernetesCallerAuthentication.KubernetesIssuer)
	require.Nil(t, config.CallerAuthentication)

	config, err = LoadConfig([]byte("logging:\n  level: info\n"))
	require.NoError(t, err)
	require.Nil(t, config.KubernetesCallerAuthentication)

	_, err = LoadConfig([]byte("kubernetesCallerAuthentication:\n  allowUnverifiedHeaders: true\n"))
	require.ErrorContains(t, err, "field allowUnverifiedHeaders not found")
}
