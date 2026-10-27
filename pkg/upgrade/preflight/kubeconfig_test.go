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

package preflight

import (
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

// Test_getClientset_UsesKUBECONFIG verifies that the preflight checks that create their own client send their
// requests to the cluster that KUBECONFIG selects, not to the cluster in ~/.kube/config.
func Test_getClientset_UsesKUBECONFIG(t *testing.T) {
	tests := []struct {
		name        string
		check       PreflightCheck
		wantRequest string
	}{
		{
			name:        "connectivity check",
			check:       NewKubernetesConnectivityCheck(""),
			wantRequest: "GET /version",
		},
		{
			name:        "resource check",
			check:       NewKubernetesResourceCheck(""),
			wantRequest: "GET /api/v1/nodes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := k8sutil.NewForbiddenAPIServer(t)
			decoy := k8sutil.NewForbiddenAPIServer(t)

			k8sutil.WriteKubeconfig(t, k8sutil.IsolateKubeconfig(t), "target", map[string]string{"target": decoy.URL})
			targetPath := filepath.Join(t.TempDir(), "target.yaml")
			k8sutil.WriteKubeconfig(t, targetPath, "target", map[string]string{"target": target.URL})
			t.Setenv(clientcmd.RecommendedConfigPathEnvVar, targetPath)

			pass, _, err := tc.check.Run(t.Context())
			require.Error(t, err)
			require.False(t, pass)
			require.Equal(t, []string{tc.wantRequest}, target.Requests())
			require.Empty(t, decoy.Requests())
		})
	}
}
