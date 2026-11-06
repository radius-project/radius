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

package kubeenv

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestEnvironmentLifecycle(t *testing.T) {
	assets, err := getKubeAssetsDir()
	require.NoError(t, err)

	environment := NewEnvironment(&envtest.Environment{BinaryAssetsDirectory: assets})
	t.Cleanup(func() { require.NoError(t, environment.Stop()) })
	cfg, err := environment.Start()
	require.NoError(t, err)
	apiAddress := environment.ControlPlane.GetAPIServer().SecureServing.URL("https", "").Host
	etcdAddress := environment.ControlPlane.Etcd.URL.Host
	certDir := environment.ControlPlane.GetAPIServer().CertDir
	dataDir := environment.ControlPlane.Etcd.DataDir

	kubeClient, err := client.New(cfg, client.Options{})
	require.NoError(t, err)
	require.NoError(t, kubeClient.List(t.Context(), &corev1.NamespaceList{}))
	require.NoError(t, environment.Stop())
	require.NoError(t, environment.Stop(), "cleanup must be idempotent")
	requireClosedEndpoint(t, apiAddress)
	requireClosedEndpoint(t, etcdAddress)
	require.NoDirExists(t, certDir)
	require.NoDirExists(t, dataDir)
}

func TestEnvironmentStartFailureCleansUp(t *testing.T) {
	assets, err := getKubeAssetsDir()
	require.NoError(t, err)

	environment := NewEnvironment(&envtest.Environment{
		BinaryAssetsDirectory: assets,
		CRDDirectoryPaths:     []string{filepath.Join(t.TempDir(), "missing-crds")},
		ErrorIfCRDPathMissing: true,
	})
	t.Cleanup(func() { require.NoError(t, environment.Stop()) })
	_, err = environment.Start()
	require.ErrorContains(t, err, "missing-crds")
	requireClosedEndpoint(t, environment.ControlPlane.GetAPIServer().SecureServing.URL("https", "").Host)
	requireClosedEndpoint(t, environment.ControlPlane.Etcd.URL.Host)
	require.NoError(t, environment.Stop())
}

func requireClosedEndpoint(t *testing.T, address string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if connection != nil {
		require.NoError(t, connection.Close())
	}
	require.Error(t, err, "envtest endpoint %s is still accepting connections", address)
}
