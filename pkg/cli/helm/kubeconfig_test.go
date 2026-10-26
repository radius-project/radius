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

package helm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	clikubernetes "github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Test_initHelmConfig_MatchesRadKubeconfigResolution verifies that rad and Helm resolve each kubeconfig
// workflow to the same cluster. See https://github.com/radius-project/radius/issues/13202.
func Test_initHelmConfig_MatchesRadKubeconfigResolution(t *testing.T) {
	const (
		homeServer      = "https://home.example.com"
		envServer       = "https://env.example.com"
		envSharedServer = "https://env-shared.example.com"
		extraServer     = "https://extra.example.com"
		partialServer   = "https://partial.example.com"
	)

	tests := []struct {
		name string
		// kubeconfig lists the files in KUBECONFIG by key. A nil value leaves KUBECONFIG unset.
		kubeconfig      []string
		contextOverride string
		wantContext     string
		wantServer      string
	}{
		{
			name:        "default kubeconfig",
			wantContext: "home-context",
			wantServer:  homeServer,
		},
		{
			name:        "custom kubeconfig in KUBECONFIG",
			kubeconfig:  []string{"env"},
			wantContext: "env-context",
			wantServer:  envServer,
		},
		{
			name:            "explicit context with the same name in the default kubeconfig",
			kubeconfig:      []string{"env"},
			contextOverride: "shared",
			wantContext:     "shared",
			wantServer:      envSharedServer,
		},
		{
			name:        "merged kubeconfig files in KUBECONFIG",
			kubeconfig:  []string{"env", "extra"},
			wantContext: "env-context",
			wantServer:  envServer,
		},
		{
			name:            "explicit context from a later merged file",
			kubeconfig:      []string{"env", "extra"},
			contextOverride: "extra-context",
			wantContext:     "extra-context",
			wantServer:      extraServer,
		},
		{
			name:        "merged partial kubeconfig files",
			kubeconfig:  []string{"partial-context", "partial-cluster"},
			wantContext: "partial",
			wantServer:  partialServer,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			paths := map[string]string{
				"env":             filepath.Join(dir, "env.yaml"),
				"extra":           filepath.Join(dir, "extra.yaml"),
				"partial-context": filepath.Join(dir, "partial-context.yaml"),
				"partial-cluster": filepath.Join(dir, "partial-cluster.yaml"),
			}
			k8sutil.WriteKubeconfig(t, k8sutil.IsolateKubeconfig(t), "home-context", map[string]string{
				"home-context": homeServer,
				"shared":       "https://home-shared.example.com",
			})
			k8sutil.WriteKubeconfig(t, paths["env"], "env-context", map[string]string{
				"env-context": envServer,
				"shared":      envSharedServer,
			})
			k8sutil.WriteKubeconfig(t, paths["extra"], "extra-context", map[string]string{
				"extra-context": extraServer,
			})
			require.NoError(t, os.WriteFile(paths["partial-context"], []byte(`apiVersion: v1
kind: Config
current-context: partial
contexts:
- name: partial
  context: {cluster: partial-cluster, user: partial-user}
`), 0600))
			require.NoError(t, os.WriteFile(paths["partial-cluster"], []byte(`apiVersion: v1
kind: Config
clusters:
- name: partial-cluster
  cluster: {server: `+partialServer+`}
users:
- name: partial-user
  user: {}
`), 0600))

			if tc.kubeconfig != nil {
				files := make([]string, 0, len(tc.kubeconfig))
				for _, key := range tc.kubeconfig {
					files = append(files, paths[key])
				}
				t.Setenv(clientcmd.RecommendedConfigPathEnvVar, strings.Join(files, string(filepath.ListSeparator)))
			}

			// rad init and rad workspace create pass the selected context name to Helm.
			selected := tc.contextOverride
			if selected == "" {
				config, err := (&clikubernetes.Impl{}).GetKubeContext()
				require.NoError(t, err)
				selected = config.CurrentContext
			}
			require.Equal(t, tc.wantContext, selected)

			// rad install kubernetes passes the --kubecontext value, which can be empty, to Helm.
			for _, kubeContext := range []string{selected, tc.contextOverride} {
				radConfig, err := clikubernetes.NewCLIClientConfig(kubeContext)
				require.NoError(t, err)
				require.Equal(t, tc.wantServer, radConfig.Host, "rad server for context %q", kubeContext)
				require.Equal(t, tc.wantServer, helmRESTConfig(t, kubeContext).Host, "Helm server for context %q", kubeContext)
			}
		})
	}
}

// Test_DefaultContourGateway_UsesKUBECONFIG verifies that the Gateway client that rad install and rad uninstall
// use sends its requests to the cluster that KUBECONFIG selects, not to the cluster in ~/.kube/config.
func Test_DefaultContourGateway_UsesKUBECONFIG(t *testing.T) {
	operations := []struct {
		name        string
		run         func(t *testing.T, kubeContext string) error
		wantRequest string
	}{
		{
			name: "install",
			run: func(t *testing.T, kubeContext string) error {
				return ensureDefaultContourGateway(t.Context(), kubeContext)
			},
			wantRequest: "GET /apis/gateway.networking.k8s.io/v1/gatewayclasses/contour",
		},
		{
			name: "uninstall",
			run: func(t *testing.T, kubeContext string) error {
				return deleteDefaultContourGateway(t.Context(), kubeContext)
			},
			wantRequest: "GET /apis/gateway.networking.k8s.io/v1/namespaces/radius-system/gateways/radius",
		},
	}

	for _, op := range operations {
		for _, explicitContext := range []bool{false, true} {
			name := op.name + " with the current context"
			if explicitContext {
				name = op.name + " with an explicit context from a later merged file"
			}

			t.Run(name, func(t *testing.T) {
				target := k8sutil.NewForbiddenAPIServer(t)
				decoy := k8sutil.NewForbiddenAPIServer(t)
				kubeContext := useTargetKubeconfig(t, target.URL, decoy.URL, explicitContext)

				err := op.run(t, kubeContext)
				require.True(t, apierrors.IsForbidden(err), "expected a Forbidden error from the target, got %v", err)
				require.Equal(t, []string{op.wantRequest}, target.Requests())
				require.Empty(t, decoy.Requests())
			})
		}
	}
}

// useTargetKubeconfig writes a decoy kubeconfig to the default path and sets KUBECONFIG to select targetURL. When
// explicitContext is true, KUBECONFIG lists a decoy file first, and the returned context selects the target from the
// later file. Otherwise the returned context is empty and the current context selects the target.
func useTargetKubeconfig(t *testing.T, targetURL string, decoyURL string, explicitContext bool) string {
	t.Helper()

	k8sutil.WriteKubeconfig(t, k8sutil.IsolateKubeconfig(t), "target", map[string]string{"target": decoyURL})

	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target.yaml")
	if !explicitContext {
		k8sutil.WriteKubeconfig(t, targetPath, "target", map[string]string{"target": targetURL})
		t.Setenv(clientcmd.RecommendedConfigPathEnvVar, targetPath)
		return ""
	}

	firstPath := filepath.Join(dir, "first.yaml")
	k8sutil.WriteKubeconfig(t, firstPath, "first", map[string]string{"first": decoyURL})
	k8sutil.WriteKubeconfig(t, targetPath, "later", map[string]string{"later": targetURL})
	t.Setenv(clientcmd.RecommendedConfigPathEnvVar, firstPath+string(filepath.ListSeparator)+targetPath)
	return "later"
}

func helmRESTConfig(t *testing.T, kubeContext string) *rest.Config {
	t.Helper()

	namespace := RadiusSystemNamespace
	helmConfig, err := initHelmConfig(&genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	})
	require.NoError(t, err)

	config, err := helmConfig.RESTClientGetter.ToRESTConfig()
	require.NoError(t, err)

	return config
}
