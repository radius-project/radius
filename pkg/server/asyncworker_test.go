/*
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
*/

package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/kubeutil"
)

const testTargetKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: target
  cluster:
    server: https://target.example:6443
    insecure-skip-tls-verify: true
contexts:
- name: target
  context:
    cluster: target
    user: target
current-context: target
users:
- name: target
  user:
    token: fake-token
`

func Test_deploymentTargetClients_NoEnvVar_ReturnsControlPlane(t *testing.T) {
	// Ensure the env var is unset for this test.
	t.Setenv(kubeutil.TargetKubeconfigEnvVar, "")

	w := &AsyncWorker{}
	controlPlane := &kubeutil.Clients{}

	got, err := w.deploymentTargetClients(context.Background(), controlPlane)
	require.NoError(t, err)
	require.Same(t, controlPlane, got, "with no target kubeconfig the control-plane clients must be returned unchanged")
}

func Test_deploymentTargetClients_ValidKubeconfig_ReturnsTargetClients(t *testing.T) {
	dir := t.TempDir()
	kubeconfigPath := filepath.Join(dir, "target.kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfigPath, []byte(testTargetKubeconfig), 0600))
	t.Setenv(kubeutil.TargetKubeconfigEnvVar, kubeconfigPath)

	w := &AsyncWorker{}
	controlPlane := &kubeutil.Clients{}

	got, err := w.deploymentTargetClients(context.Background(), controlPlane)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotSame(t, controlPlane, got, "with a target kubeconfig set, distinct target clients must be returned")
}

func Test_deploymentTargetClients_UnreadableKubeconfig_ReturnsError(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "does-not-exist.kubeconfig")
	t.Setenv(kubeutil.TargetKubeconfigEnvVar, missingPath)

	w := &AsyncWorker{}

	_, err := w.deploymentTargetClients(context.Background(), &kubeutil.Clients{})
	require.Error(t, err)
	require.Contains(t, err.Error(), kubeutil.TargetKubeconfigEnvVar)
	require.Contains(t, err.Error(), missingPath)
}
