/*
------------------------------------------------------------
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
------------------------------------------------------------
*/

package kubeutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

func TestBuildConfigOptions(t *testing.T) {
	optionTests := []struct {
		name string
		in   *ConfigOptions
		out  *ConfigOptions
	}{
		{
			name: "nil",
			in:   nil,
			out: &ConfigOptions{
				ConfigFilePath: clientcmd.RecommendedHomeFile,
				QPS:            0.0,
				Burst:          0,
			},
		},
		{
			name: "only QPS",
			in: &ConfigOptions{
				ConfigFilePath: "custom",
				QPS:            DefaultServerQPS,
			},
			out: &ConfigOptions{
				ConfigFilePath: "custom",
				QPS:            DefaultServerQPS,
				Burst:          0,
			},
		},
		{
			name: "only Burst",
			in: &ConfigOptions{
				ConfigFilePath: "custom",
				Burst:          DefaultServerBurst,
			},
			out: &ConfigOptions{
				ConfigFilePath: "custom",
				QPS:            0.0,
				Burst:          DefaultServerBurst,
			},
		},
	}

	for _, tc := range optionTests {
		t.Run(tc.name, func(t *testing.T) {
			result := buildConfigOptions(tc.in)
			require.Equal(t, tc.out, result)
		})
	}
}

func TestNewClientConfig(t *testing.T) {
	configFile, _ := os.CreateTemp("", "")
	defer os.Remove(configFile.Name())

	err := os.WriteFile(configFile.Name(), []byte(`
kind: Config
apiVersion: v1
clusters:
- cluster:
    api-version: v1
    server: https://kubernetes.default.svc:443
  name: kubeconfig-cluster
contexts:
- context:
    cluster: kubeconfig-cluster
    namespace: default
    user: kubeconfig-user
  name: kubeconfig-context
current-context: kubeconfig-context
users:
- name: kubeconfig-user
`), os.FileMode(0755))
	require.NoError(t, err)

	optionTests := []struct {
		name string
		in   *ConfigOptions
		out  *ConfigOptions
	}{
		{
			name: "default",
			in: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
			},
			out: &ConfigOptions{
				QPS:   0.0,
				Burst: 0,
			},
		},
		{
			name: "only QPS",
			in: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
				QPS:            DefaultServerQPS,
			},
			out: &ConfigOptions{
				QPS:   DefaultServerQPS,
				Burst: 0,
			},
		},
		{
			name: "only Burst",
			in: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
				Burst:          DefaultServerBurst,
			},
			out: &ConfigOptions{
				QPS:   0.0,
				Burst: DefaultServerBurst,
			},
		},
		{
			name: "QPS and Burst",
			in: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
				QPS:            DefaultServerQPS,
				Burst:          DefaultServerBurst,
			},
			out: &ConfigOptions{
				QPS:   DefaultServerQPS,
				Burst: DefaultServerBurst,
			},
		},
	}

	for _, tc := range optionTests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := NewClientConfig(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.out.QPS, cfg.QPS)
			require.Equal(t, tc.out.Burst, cfg.Burst)
		})
	}
}
