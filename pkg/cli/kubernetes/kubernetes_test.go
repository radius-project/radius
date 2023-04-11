// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

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

	emptyConfig, _ := os.CreateTemp("", "")
	defer os.Remove(emptyConfig.Name())

	err = os.WriteFile(emptyConfig.Name(), []byte(`
kind: Config
apiVersion: v1
clusters:
- cluster:
    api-version: v1
    server: https://kubernetes.default.svc:443
  name: kubeconfig-cluster
contexts:
users:
- name: kubeconfig-user
`), os.FileMode(0755))
	require.NoError(t, err)

	tests := []struct {
		name       string
		configFile string
		in         string
		out        string
		err        error
	}{
		{
			name:       "get kubeconfig-context",
			configFile: configFile.Name(),
			in:         "kubeconfig-context",
			out:        "kubeconfig-context",
			err:        nil,
		},
		{
			name:       "get default context",
			configFile: configFile.Name(),
			in:         "",
			out:        "kubeconfig-context",
			err:        nil,
		},
		{
			name:       "get kubeconfig-test",
			configFile: configFile.Name(),
			in:         "kubeconfig-test",
			out:        "kubeconfig-test",
			err:        nil,
		},
		{
			name:       "try to get non-existing context",
			configFile: configFile.Name(),
			in:         "unknown",
			err:        errors.New("kubernetes context 'unknown' could not be found"),
		},
		{
			name:       "no specified context",
			configFile: emptyConfig.Name(),
			in:         "",
			err:        errors.New("no kubernetes context is set"),
		},
		{
			name:       "try to get non-existing config file",
			configFile: "non-existing",
			in:         "",
			err:        errors.New("open non-existing: no such file or directory"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contextName, err := GetContextFromConfigFileIfExists(tc.configFile, tc.in)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.out, contextName)
			}
		})
	}
}
