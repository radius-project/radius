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

package providers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/recipes/kubernetes/clusteraccess"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

func TestKubernetesProvider_BuildConfig(t *testing.T) {
	expectedConfig := map[string]any{
		"config_path": clientcmd.RecommendedHomeFile,
	}

	p := newKubernetesProvider(clusteraccess.NewResolver())
	config, err := p.BuildConfig(testcontext.New(t), nil)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, config)
}

func TestKubernetesProvider_BuildConfig_Error(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "testvalue")
	t.Setenv("KUBERNETES_SERVICE_PORT", "1111")

	p := newKubernetesProvider(clusteraccess.NewResolver())
	config, err := p.BuildConfig(testcontext.New(t), nil)
	require.Error(t, err)
	require.Nil(t, config)
}

func TestKubernetesProvider_BuildConfig_InjectedTargetKubeconfig(t *testing.T) {
	// When RADIUS_TARGET_KUBECONFIG is set, the provider targets that kubeconfig
	// regardless of whether the process is running in-cluster.
	kubeconfigPath := filepath.Join(t.TempDir(), "target.kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\nkind: Config\n"), 0600))

	t.Setenv("KUBERNETES_SERVICE_HOST", "testvalue")
	t.Setenv("KUBERNETES_SERVICE_PORT", "1111")
	t.Setenv(clusteraccess.TargetKubeconfigEnvVar, kubeconfigPath)

	expectedConfig := map[string]any{
		"config_path": kubeconfigPath,
	}

	p := newKubernetesProvider(clusteraccess.NewResolver())
	config, err := p.BuildConfig(testcontext.New(t), nil)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, config)
}

func TestKubernetesProvider_BuildConfig_InjectedTargetKubeconfigMissing(t *testing.T) {
	// A configured but unreadable injected kubeconfig must fail loudly, naming the
	// env var and path, rather than deferring a less actionable error to Terraform.
	missingPath := filepath.Join(t.TempDir(), "does-not-exist.kubeconfig")

	t.Setenv("KUBERNETES_SERVICE_HOST", "testvalue")
	t.Setenv("KUBERNETES_SERVICE_PORT", "1111")
	t.Setenv(clusteraccess.TargetKubeconfigEnvVar, missingPath)

	p := newKubernetesProvider(clusteraccess.NewResolver())
	config, err := p.BuildConfig(testcontext.New(t), nil)
	require.Error(t, err)
	require.Nil(t, config)
	require.Contains(t, err.Error(), clusteraccess.TargetKubeconfigEnvVar)
	require.Contains(t, err.Error(), missingPath)
}
