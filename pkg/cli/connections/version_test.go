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

package connections

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

func Test_GetControlPlaneVersion_WorkspaceTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/apis/api.ucp.dev/v1alpha3/version", r.URL.Path)
		_, _ = io.WriteString(w, `{"release":"0.60.2"}`)
	}))
	defer server.Close()
	kubeconfig := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(kubeconfig, []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
contexts:
- name: selected
  context: {cluster: selected, user: selected}
clusters:
- name: selected
  cluster: {server: %s}
users:
- name: selected
  user: {}
`, server.URL)), 0600))
	useTestKubeconfig(t, kubeconfig)
	info, err := DefaultFactory.GetControlPlaneVersion(t.Context(), workspaces.Workspace{
		Connection: map[string]any{"kind": "kubernetes", "context": "selected"},
	})
	require.NoError(t, err)
	require.Equal(t, "0.60.2", info.Release)
}

func Test_GetControlPlaneVersion_InvalidWorkspace(t *testing.T) {
	_, err := DefaultFactory.GetControlPlaneVersion(t.Context(), workspaces.Workspace{})
	require.ErrorContains(t, err, "missing required field")
}

func Test_GetControlPlaneVersion_InvalidKubernetesContext(t *testing.T) {
	kubeconfig := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(kubeconfig, []byte(`apiVersion: v1
kind: Config
contexts: []
clusters: []
users: []
`), 0600))
	useTestKubeconfig(t, kubeconfig)
	_, err := DefaultFactory.GetControlPlaneVersion(t.Context(), workspaces.Workspace{
		Connection: map[string]any{"kind": "kubernetes", "context": "not-present"},
	})
	require.Error(t, err)
}

func useTestKubeconfig(t *testing.T, path string) {
	t.Helper()
	previous := clientcmd.RecommendedHomeFile
	clientcmd.RecommendedHomeFile = path
	t.Cleanup(func() { clientcmd.RecommendedHomeFile = previous })
}
