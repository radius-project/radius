// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubeutil

import (
	"errors"
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

func TestGetContextFromConfigFileIfExists(t *testing.T) {
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
- context:
    cluster: kubeconfig-test
    namespace: default
    user: kubeconfig-user
  name: kubeconfig-test
current-context: kubeconfig-context
users:
- name: kubeconfig-user
`), os.FileMode(0755))
	require.NoError(t, err)

	tests := []struct {
		name string
		opt  *ConfigOptions
		err  error
	}{
		{
			name: "get kubeconfig-context",
			opt: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
				ContextName:    "kubeconfig-context",
			},
			err: nil,
		},
		{
			name: "get kubeconfig-test",
			opt: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
				ContextName:    "kubeconfig-test",
			},
			err: nil,
		},
		{
			name: "not exist",
			opt: &ConfigOptions{
				ConfigFilePath: configFile.Name(),
				ContextName:    "unknown",
			},
			err: errors.New("kubernetes context 'unknown' could not be found"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contextName, context, err := GetContextFromConfigFileIfExists(tc.opt)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
				require.NotNil(t, context)
				require.Equal(t, tc.opt.ContextName, contextName)
			}
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
