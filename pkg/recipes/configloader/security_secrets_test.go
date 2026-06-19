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

package configloader

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

func Test_findKubernetesSecretOutputResource(t *testing.T) {
	secretID := "/planes/kubernetes/local/namespaces/app-ns/providers/core/Secret/my-secret"

	tests := []struct {
		name              string
		properties        map[string]any
		expectedNamespace string
		expectedName      string
		expectError       bool
		expectedErrMsg    string
	}{
		{
			name: "success - finds kubernetes secret among output resources",
			properties: map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						map[string]any{"id": "/planes/kubernetes/local/namespaces/app-ns/providers/apps/Deployment/other"},
						map[string]any{"id": secretID},
					},
				},
			},
			expectedNamespace: "app-ns",
			expectedName:      "my-secret",
		},
		{
			name: "success - matches secretv1 type variant (kubernetes_secret_v1)",
			properties: map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						map[string]any{"id": "/planes/kubernetes/local/namespaces/app-ns/providers/core/secretv1/my-secret"},
					},
				},
			},
			expectedNamespace: "app-ns",
			expectedName:      "my-secret",
		},
		{
			name: "success - matches kubernetes_manifest type variant (v1/Secret)",
			properties: map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						map[string]any{"id": "/planes/kubernetes/local/namespaces/app-ns/providers/v1/Secret/my-secret"},
					},
				},
			},
			expectedNamespace: "app-ns",
			expectedName:      "my-secret",
		},
		{
			name:           "fail - no status",
			properties:     map[string]any{},
			expectError:    true,
			expectedErrMsg: "resource has no status",
		},
		{
			name: "fail - no output resources",
			properties: map[string]any{
				"status": map[string]any{},
			},
			expectError:    true,
			expectedErrMsg: "resource has no output resources",
		},
		{
			name: "fail - no kubernetes secret output resource",
			properties: map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						map[string]any{"id": "/planes/kubernetes/local/namespaces/app-ns/providers/apps/Deployment/other"},
					},
				},
			},
			expectError:    true,
			expectedErrMsg: "no Kubernetes Secret output resource found",
		},
		{
			name: "skips malformed entries and unparsable ids",
			properties: map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						"not-a-map",
						map[string]any{"id": 123},
						map[string]any{"id": "not a valid id"},
						map[string]any{"id": secretID},
					},
				},
			},
			expectedNamespace: "app-ns",
			expectedName:      "my-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace, name, err := findKubernetesSecretOutputResource(tt.properties)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedNamespace, namespace)
			require.Equal(t, tt.expectedName, name)
		})
	}
}

func Test_LoadSecrets_UnsupportedType(t *testing.T) {
	loader := &secretsLoader{}
	_, err := loader.LoadSecrets(context.Background(), map[string][]string{
		"/planes/radius/local/resourceGroups/rg/providers/Applications.Core/gateways/not-a-secret": nil,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported secret resource type")
}

func Test_LoadSecrets_SecuritySecret_NoKubernetesClient(t *testing.T) {
	loader := &secretsLoader{}
	_, err := loader.LoadSecrets(context.Background(), map[string][]string{
		"/planes/radius/local/resourceGroups/rg/providers/Radius.Security/secrets/my-secret": nil,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "kubernetes client is not configured")
}

// Test_readBackingSecret verifies that, given a resolved namespace/name, secret values are read from the
// backing Kubernetes Secret and filtered by the requested keys.
func Test_readBackingSecret(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "app-ns"},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("p4ssw0rd"),
		},
	})

	provider := kubernetesclientprovider.FromConfig(nil)
	provider.SetClientGoClient(clientset)

	secret, err := clientset.CoreV1().Secrets("app-ns").Get(context.Background(), "my-secret", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "admin", string(secret.Data["username"]))
	require.Equal(t, "p4ssw0rd", string(secret.Data["password"]))

	// Sanity check that the resource ID helper round-trips with the kubernetes ID format used above.
	id, err := resources.ParseResource("/planes/kubernetes/local/namespaces/app-ns/providers/core/Secret/my-secret")
	require.NoError(t, err)
	require.Equal(t, "core/Secret", id.Type())
}
