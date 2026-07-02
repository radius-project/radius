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
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	genfake "github.com/radius-project/radius/pkg/cli/clients_new/generated/fake"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/to"
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
		{
			name: "skips cluster-scoped secret with empty namespace and continues to namespaced secret",
			properties: map[string]any{
				"status": map[string]any{
					"outputResources": []any{
						map[string]any{"id": "/planes/kubernetes/local/providers/core/Secret/cluster-secret"},
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

// Test_loadSecuritySecret drives the production read path for a Radius.Security/secrets resource: it fetches the
// resource through a fake generated client to extract the secret kind and locate the backing Kubernetes Secret,
// then reads and filters the secret values from a fake clientset. It covers explicit-kind reads, the default-kind
// path with an empty key filter, and the missing-key error.
func Test_loadSecuritySecret(t *testing.T) {
	const (
		secretResourceID = "/planes/radius/local/resourceGroups/rg/providers/Radius.Security/secrets/my-secret"
		backingSecretID  = "/planes/kubernetes/local/namespaces/app-ns/providers/core/Secret/my-secret"
	)

	// backingSecretProperties builds the resource properties returned by the fake Get, including the
	// status.outputResources entry that points loadSecuritySecret at the backing Kubernetes Secret. An empty
	// kind omits the `kind` property so the default-kind path is exercised.
	backingSecretProperties := func(kind string) map[string]any {
		props := map[string]any{
			"status": map[string]any{
				"outputResources": []any{
					map[string]any{"id": backingSecretID},
				},
			},
		}
		if kind != "" {
			props["kind"] = kind
		}
		return props
	}

	tests := []struct {
		name           string
		properties     map[string]any
		keysFilter     []string
		expectedType   string
		expectedData   map[string]string
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:         "success - explicit kind filters to requested keys",
			properties:   backingSecretProperties("basicAuthentication"),
			keysFilter:   []string{"username"},
			expectedType: "basicAuthentication",
			expectedData: map[string]string{"username": "admin"},
		},
		{
			name:         "success - default kind returns all keys when filter is empty",
			properties:   backingSecretProperties(""),
			keysFilter:   nil,
			expectedType: defaultSecuritySecretKind,
			expectedData: map[string]string{"username": "admin", "password": "p4ssw0rd"},
		},
		{
			name:           "fail - requested key missing from backing secret",
			properties:     backingSecretProperties("generic"),
			keysFilter:     []string{"missing"},
			expectError:    true,
			expectedErrMsg: "'missing' secret key was not found",
		},
		{
			name:           "fail - resource has no properties",
			properties:     nil,
			expectError:    true,
			expectedErrMsg: "has no properties",
		},
		{
			name: "fail - no backing kubernetes secret output resource",
			properties: map[string]any{
				"kind":   "generic",
				"status": map[string]any{"outputResources": []any{}},
			},
			expectError:    true,
			expectedErrMsg: "failed to locate backing Kubernetes Secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clientset := k8sfake.NewSimpleClientset(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "app-ns"},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("p4ssw0rd"),
				},
			})

			provider := kubernetesclientprovider.FromConfig(nil)
			provider.SetClientGoClient(clientset)

			loader := &secretsLoader{
				ArmClientOptions: &arm.ClientOptions{
					ClientOptions: policy.ClientOptions{
						Transport: genfake.NewServerFactoryTransport(&genfake.ServerFactory{
							GenericResourcesServer: genfake.GenericResourcesServer{
								Get: func(ctx context.Context, resourceName string, options *generated.GenericResourcesClientGetOptions) (resp azfake.Responder[generated.GenericResourcesClientGetResponse], errResp azfake.ErrorResponder) {
									require.Equal(t, "my-secret", resourceName)
									resp.SetResponse(http.StatusOK, generated.GenericResourcesClientGetResponse{
										GenericResource: generated.GenericResource{
											ID:         to.Ptr(secretResourceID),
											Name:       to.Ptr("my-secret"),
											Properties: tt.properties,
										},
									}, nil)
									return
								},
							},
						}),
					},
				},
				KubernetesProvider: provider,
			}

			id, err := resources.ParseResource(secretResourceID)
			require.NoError(t, err)

			secretData, err := loader.loadSecuritySecret(context.Background(), id, tt.keysFilter)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedType, secretData.Type)
			require.Equal(t, tt.expectedData, secretData.Data)
		})
	}
}
