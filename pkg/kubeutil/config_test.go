// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubeutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

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

	cfg, err := NewClusterConfig(configFile.Name())
	require.NoError(t, err)
	require.Equal(t, DefaultQPS, cfg.QPS)
	require.Equal(t, DefaultBurst, cfg.Burst)

	cfg, err = NewClusterConfigWithContext(configFile.Name(), "", false)
	require.NoError(t, err)
	require.Equal(t, DefaultQPS, cfg.QPS)
	require.Equal(t, DefaultBurst, cfg.Burst)
}
