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

package kubernetes

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
)

func TestEnsureNamespace(t *testing.T) {
	f := k8sfake.NewClientset(&v1.Namespace{Name: "radius-test"})

	k8sutil.PrependPatchReactor(f, "namespaces", func(pa clienttesting.PatchAction) runtime.Object {
		return &v1.Namespace{
			Name: pa.GetName(),
		}
	})

	ctx := t.Context()
	err := EnsureNamespace(ctx, f, "radius-test")
	require.NoError(t, err)
	_, err = f.CoreV1().Namespaces().Get(ctx, "radius-test", meta_v1.GetOptions{})
	require.NoError(t, err)
}

func TestDeleteNamespace(t *testing.T) {
	namespace := "radius-test"
	f := k8sfake.NewClientset(&v1.Namespace{Name: namespace})

	ctx := t.Context()
	_, err := f.CoreV1().Namespaces().Get(ctx, namespace, meta_v1.GetOptions{})
	require.NoError(t, err)
	err = deleteNamespace(ctx, f, namespace)
	require.NoError(t, err)
	_, err = f.CoreV1().Namespaces().Get(ctx, namespace, meta_v1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err), "expected not found error but got %v", err)
}

func TestGetContextFromConfigFileIfExists(t *testing.T) {
	configFile, err := os.CreateTemp("", "")
	require.NoError(t, err)
	defer os.Remove(configFile.Name())

	err = os.WriteFile(configFile.Name(), []byte(`
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

	emptyConfig, err := os.CreateTemp("", "")
	require.NoError(t, err)
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

// Test_GetKubeContext_UsesKUBECONFIG verifies that GetKubeContext reads the kubeconfig that KUBECONFIG selects
// instead of ~/.kube/config, as Helm does. See https://github.com/radius-project/radius/issues/13202.
func Test_GetKubeContext_UsesKUBECONFIG(t *testing.T) {
	useEnvKubeconfig(t)

	config, err := (&Impl{}).GetKubeContext()
	require.NoError(t, err)
	require.Equal(t, "env-context", config.CurrentContext)
	require.NotContains(t, config.Contexts, "home-context")
}

// Test_GetContextFromConfigFileIfExists_UsesKUBECONFIG verifies that GetContextFromConfigFileIfExists reads the
// kubeconfig that KUBECONFIG selects when no path is given. See https://github.com/radius-project/radius/issues/13202.
func Test_GetContextFromConfigFileIfExists_UsesKUBECONFIG(t *testing.T) {
	explicitPath := useEnvKubeconfig(t)

	t.Run("default kubeconfig", func(t *testing.T) {
		contextName, err := GetContextFromConfigFileIfExists("", "")
		require.NoError(t, err)
		require.Equal(t, "env-context", contextName)

		_, err = GetContextFromConfigFileIfExists("", "home-context")
		require.ErrorContains(t, err, "kubernetes context 'home-context' could not be found")
	})

	t.Run("explicit path ignores KUBECONFIG", func(t *testing.T) {
		contextName, err := GetContextFromConfigFileIfExists(explicitPath, "")
		require.NoError(t, err)
		require.Equal(t, "explicit-context", contextName)
	})
}

// useEnvKubeconfig writes a home kubeconfig with home-context and points KUBECONFIG at a file with env-context. It
// returns the path of a third kubeconfig, with explicit-context, that KUBECONFIG does not list.
func useEnvKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	k8sutil.WriteKubeconfig(t, k8sutil.IsolateKubeconfig(t), "home-context", map[string]string{
		"home-context": "https://home.example.com",
	})
	envPath := filepath.Join(dir, "env.yaml")
	k8sutil.WriteKubeconfig(t, envPath, "env-context", map[string]string{"env-context": "https://env.example.com"})
	explicitPath := filepath.Join(dir, "explicit.yaml")
	k8sutil.WriteKubeconfig(t, explicitPath, "explicit-context", map[string]string{
		"explicit-context": "https://explicit.example.com",
	})
	t.Setenv(clientcmd.RecommendedConfigPathEnvVar, envPath)
	return explicitPath
}
