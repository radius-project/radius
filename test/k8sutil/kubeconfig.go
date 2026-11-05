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

package k8sutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// IsolateKubeconfig makes kubeconfig loading ignore the user's real kubeconfig files for the rest of the test. It
// unsets KUBECONFIG, sets HOME to a new temporary directory, and points clientcmd.RecommendedHomeFile into that
// directory. It returns the new default kubeconfig path, which does not exist until the test writes it.
//
// client-go sets clientcmd.RecommendedHomeFile once, when the package loads, so setting HOME alone does not
// change the default path. Tests that call IsolateKubeconfig must not call t.Parallel.
func IsolateKubeconfig(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(clientcmd.RecommendedConfigPathEnvVar, "")

	previous := clientcmd.RecommendedHomeFile
	clientcmd.RecommendedHomeFile = filepath.Join(home, clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName)
	t.Cleanup(func() { clientcmd.RecommendedHomeFile = previous })

	return clientcmd.RecommendedHomeFile
}

// WriteKubeconfig writes a kubeconfig file to path and creates its parent directory. Each key in servers is the
// name of a context, cluster, and user, and its value is the server URL of that cluster.
func WriteKubeconfig(t *testing.T, path string, currentContext string, servers map[string]string) {
	t.Helper()

	config := clientcmdapi.NewConfig()
	for name, server := range servers {
		config.Clusters[name] = &clientcmdapi.Cluster{Server: server}
		config.AuthInfos[name] = &clientcmdapi.AuthInfo{}
		config.Contexts[name] = &clientcmdapi.Context{Cluster: name, AuthInfo: name}
	}
	config.CurrentContext = currentContext

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, clientcmd.WriteToFile(*config, path))
}

// ForbiddenAPIServer is a fake Kubernetes API server that records each request and rejects it with 403 Forbidden.
// Tests use it to check which cluster a client sends requests to. The client cannot change anything.
type ForbiddenAPIServer struct {
	*httptest.Server

	mu       sync.Mutex
	requests []string
}

// NewForbiddenAPIServer starts a ForbiddenAPIServer and closes it when the test ends.
func NewForbiddenAPIServer(t *testing.T) *ForbiddenAPIServer {
	t.Helper()

	s := &ForbiddenAPIServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403}`)
	}))
	t.Cleanup(s.Close)

	return s
}

// Requests returns the method and path of each request that the server received, in order.
func (s *ForbiddenAPIServer) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.requests)
}
