// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
			},
		},
		{
			name: "only QPS",
			in: &ConfigOptions{
				ConfigFilePath: "custom",
				QPS:            ServerQPS,
			},
			out: &ConfigOptions{
				ConfigFilePath: "custom",
				QPS:            ServerQPS,
			},
		},
		{
			name: "only Burst",
			in: &ConfigOptions{
				ConfigFilePath: "custom",
				Burst:          ServerBurst,
			},
			out: &ConfigOptions{
				ConfigFilePath: "custom",
				Burst:          ServerBurst,
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

func TestNewClusterConfig(t *testing.T) {
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
			name: "only QPS",
			in: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
				QPS:            ServerQPS,
			},
			out: &ConfigOptions{
				QPS: ServerQPS,
			},
		},
		{
			name: "only Burst",
			in: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
				Burst:          ServerBurst,
			},
			out: &ConfigOptions{
				Burst: ServerBurst,
			},
		},
		{
			name: "QPS and Burst",
			in: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
				QPS:            ServerQPS,
				Burst:          ServerBurst,
			},
			out: &ConfigOptions{
				QPS:   ServerQPS,
				Burst: ServerBurst,
			},
		},
	}

	for _, tc := range optionTests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := NewClusterConfig(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.out.QPS, cfg.QPS)
			require.Equal(t, tc.out.Burst, cfg.Burst)
		})
	}
}
