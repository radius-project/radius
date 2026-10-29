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

package processors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v4"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/azure/armauth"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	ARMResourceID                    = "/subscriptions/0000/resourceGroups/test-rg/providers/Microsoft.Compute/virtualMachines/test-vm"
	ARMProviderPath                  = "/subscriptions/0000/providers/Microsoft.Compute"
	AzureUCPResourceID               = "/planes/azure/azurecloud/subscriptions/0000/resourceGroups/test-rg/providers/Microsoft.Compute/virtualMachines/test-vm"
	ARMAPIVersion                    = "2020-01-01"
	AWSResourceID                    = "/planes/aws/aws/accounts/0000/regions/us-east-1/providers/AWS.Kinesis/Streams/test-stream"
	KubernetesCoreGroupResourceID    = "/planes/kubernetes/local/namespaces/test-namespace/providers/core/Secret/test-name"
	KubernetesNonCoreGroupResourceID = "/planes/kubernetes/local/namespaces/test-namespace/providers/apps/Deployment/test-name"

	// ARMExtensionResourceID is an Azure extension resource (a Microsoft.Authorization/locks resource
	// attached to a Microsoft.DocumentDB/databaseAccounts resource), as described in
	// https://github.com/radius-project/radius/issues/12694.
	ARMExtensionResourceID   = "/subscriptions/0000/resourceGroups/test-rg/providers/Microsoft.DocumentDB/databaseAccounts/test-account/providers/Microsoft.Authorization/locks/test-lock"
	ARMExtensionProviderPath = "/subscriptions/0000/providers/Microsoft.Authorization"
)

func Test_Delete_InvalidResourceID(t *testing.T) {
	c := NewResourceClient(nil, nil, nil)
	err := c.Delete(t.Context(), "invalid")
	require.Error(t, err)
}

func Test_NewResourceClientFactory(t *testing.T) {
	controlPlaneProvider := kubernetesclientprovider.FromConfig(&rest.Config{Host: "https://control-plane.example.com"})
	factory := NewResourceClientFactory(nil, nil, controlPlaneProvider)

	t.Run("uses control-plane provider when target config is nil", func(t *testing.T) {
		client, ok := factory(nil).(*resourceClient)
		require.True(t, ok)
		require.Same(t, controlPlaneProvider, client.kubernetesClient)
	})

	t.Run("uses tuned copy of target config", func(t *testing.T) {
		targetConfig := &rest.Config{
			Host:  "https://target.example.com",
			QPS:   1,
			Burst: 2,
		}

		client, ok := factory(targetConfig).(*resourceClient)
		require.True(t, ok)
		require.NotSame(t, controlPlaneProvider, client.kubernetesClient)
		require.NotSame(t, targetConfig, client.kubernetesClient.Config())
		require.Equal(t, targetConfig.Host, client.kubernetesClient.Config().Host)
		require.Equal(t, kubeutil.DefaultServerQPS, client.kubernetesClient.Config().QPS)
		require.Equal(t, kubeutil.DefaultServerBurst, client.kubernetesClient.Config().Burst)
		require.Equal(t, float32(1), targetConfig.QPS)
		require.Equal(t, 2, targetConfig.Burst)
	})
}

func Test_Delete_ARM(t *testing.T) {
	t.Run("failure - delete fails", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMResourceID, handleJSONResponse(t, v1.ErrorResponse{
			Error: &v1.ErrorDetails{
				Code: v1.CodeConflict,
			},
		}, 409))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(t.Context(), ARMResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
	})

	t.Run("success - lookup API Version (default)", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMResourceID, handleDeleteSuccess())
		mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace: new("Microsoft.Compute"),
			ResourceTypes: []*armresources.ProviderResourceType{
				{
					ResourceType: new("anotherType"),
					APIVersions:  []*string{},
				},
				{
					ResourceType:      new("virtualMachines"),
					DefaultAPIVersion: new(ARMAPIVersion),
					APIVersions:       []*string{new(ARMAPIVersion)},
				},
			},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(t.Context(), ARMResourceID)
		require.NoError(t, err)
	})

	t.Run("success - lookup API Version (first available)", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMResourceID, handleDeleteSuccess())
		mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace: new("Microsoft.Compute"),
			ResourceTypes: []*armresources.ProviderResourceType{
				{
					ResourceType: new("virtualMachines"),
					APIVersions:  []*string{new("2020-01-01"), new("2020-01-02")},
				},
			},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(t.Context(), ARMResourceID)
		require.NoError(t, err)
	})

	t.Run("success - deletion returns 404", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMResourceID, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				handleDeleteSuccess()(w, r)
				return
			}
			handleNotFound(t)(w, r)
		})
		mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace: new("Microsoft.Compute"),
			ResourceTypes: []*armresources.ProviderResourceType{
				{
					ResourceType: new("anotherType"),
					APIVersions:  []*string{},
				},
				{
					ResourceType:      new("virtualMachines"),
					DefaultAPIVersion: new(ARMAPIVersion),
					APIVersions:       []*string{new(ARMAPIVersion)},
				},
			},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(t.Context(), ARMResourceID)
		require.NoError(t, err)
	})

	t.Run("failure - lookup API Version - provider not found", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMProviderPath, handleNotFound(t))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(t.Context(), ARMResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
	})

	t.Run("failure - lookup API Version - resource type not found", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace:     new("Microsoft.Compute"),
			ResourceTypes: []*armresources.ProviderResourceType{},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(t.Context(), ARMResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
		require.Contains(t, err.Error(), "could not find API version for type \"Microsoft.Compute/virtualMachines\", type was not found")
	})

	t.Run("failure - lookup API Version - no api versions", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace: new("Microsoft.Compute"),
			ResourceTypes: []*armresources.ProviderResourceType{
				{
					ResourceType: new("virtualMachines"),
					APIVersions:  []*string{},
				},
			},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(t.Context(), ARMResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
		require.Contains(t, err.Error(), "could not find API version for type \"Microsoft.Compute/virtualMachines\", no supported API versions")
	})

	t.Run("success - lookup API Version - extension resource", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMExtensionResourceID, handleDeleteSuccess())
		mux.HandleFunc(ARMExtensionProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace: new("Microsoft.Authorization"),
			ResourceTypes: []*armresources.ProviderResourceType{
				{
					ResourceType:      new("locks"),
					DefaultAPIVersion: new(ARMAPIVersion),
					APIVersions:       []*string{new(ARMAPIVersion)},
				},
			},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(t.Context(), ARMExtensionResourceID)
		require.NoError(t, err)
	})
}

func Test_Delete_ARM_NestedChildTypeFallback(t *testing.T) {
	const (
		shareID          = "/subscriptions/0000/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/account/fileServices/default/shares/share"
		storagePath      = "/subscriptions/0000/providers/Microsoft.Storage"
		mongoID          = "/subscriptions/0000/resourceGroups/test-rg/providers/Microsoft.DocumentDB/databaseAccounts/account/mongodbDatabases/db"
		documentDBPath   = "/subscriptions/0000/providers/Microsoft.DocumentDB"
		extensionChildID = "/subscriptions/0000/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/account/providers/Microsoft.Test/parents/parent/children/child"
		testProviderPath = "/subscriptions/0000/providers/Microsoft.Test"
	)

	resourceType := func(name string, versions ...string) *armresources.ProviderResourceType {
		rt := &armresources.ProviderResourceType{ResourceType: new(name)}
		for _, version := range versions {
			rt.APIVersions = append(rt.APIVersions, new(version))
		}
		return rt
	}

	tests := []struct {
		name            string
		id              string
		providerPath    string
		types           []*armresources.ProviderResourceType
		rejectedVersion string
		wantVersion     string
		wantErr         string
	}{
		{
			name:         "nested child absent from metadata uses top-level type",
			id:           shareID,
			providerPath: storagePath,
			types:        []*armresources.ProviderResourceType{resourceType("storageAccounts", "top-level")},
			wantVersion:  "top-level",
		},
		{
			name:            "missing child type retries next parent version after read rejection",
			id:              shareID,
			providerPath:    storagePath,
			types:           []*armresources.ProviderResourceType{resourceType("storageAccounts", "2026-10-01", "2025-08-01")},
			rejectedVersion: "2026-10-01",
			wantVersion:     "2025-08-01",
		},
		{
			name:         "nearest listed parent is preferred",
			id:           shareID,
			providerPath: storagePath,
			types: []*armresources.ProviderResourceType{
				resourceType("storageAccounts", "top-level"),
				resourceType("storageAccounts/fileServices", "parent"),
			},
			wantVersion: "parent",
		},
		{
			name:         "exact child type is preferred",
			id:           shareID,
			providerPath: storagePath,
			types: []*armresources.ProviderResourceType{
				resourceType("storageAccounts", "top-level"),
				resourceType("storageAccounts/fileServices/shares", "exact"),
			},
			wantVersion: "exact",
		},
		{
			name:         "single-level child absent from metadata uses parent type",
			id:           mongoID,
			providerPath: documentDBPath,
			types:        []*armresources.ProviderResourceType{resourceType("databaseAccounts", "top-level")},
			wantVersion:  "top-level",
		},
		{
			name:         "extension child absent from metadata uses extension parent type",
			id:           extensionChildID,
			providerPath: testProviderPath,
			types:        []*armresources.ProviderResourceType{resourceType("parents", "extension-parent")},
			wantVersion:  "extension-parent",
		},
		{
			name:         "no listed type or parent",
			id:           shareID,
			providerPath: storagePath,
			types:        []*armresources.ProviderResourceType{resourceType("blobServices", "unrelated")},
			wantErr:      `could not find API version for type "Microsoft.Storage/storageAccounts/fileServices/shares", type was not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls []string
			var mu sync.Mutex
			mux := http.NewServeMux()
			mux.HandleFunc(tt.providerPath, handleJSONResponse(t, armresources.Provider{ResourceTypes: tt.types}, http.StatusOK))
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				version := r.URL.Query().Get("api-version")
				calls = append(calls, r.Method+" "+r.URL.Path+" "+version)
				if r.URL.Path != tt.id {
					http.NotFound(w, r)
					return
				}
				if r.Method == http.MethodGet && version == tt.rejectedVersion {
					handleJSONResponse(t, v1.ErrorResponse{
						Error: &v1.ErrorDetails{Code: "InvalidApiVersionParameter"},
					}, http.StatusBadRequest)(w, r)
					return
				}
				handleDeleteSuccess()(w, r)
			})

			server := httptest.NewServer(mux)
			defer server.Close()

			c := NewResourceClient(newArmOptions(server.URL), nil, nil)
			c.armClientOptions = newClientOptions(server.Client(), server.URL)

			err := c.Delete(t.Context(), tt.id)
			mu.Lock()
			defer mu.Unlock()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, calls)
				return
			}

			require.NoError(t, err)
			var want []string
			if tt.rejectedVersion != "" {
				want = append(want, "GET "+tt.id+" "+tt.rejectedVersion)
			}
			want = append(want, "GET "+tt.id+" "+tt.wantVersion, "DELETE "+tt.id+" "+tt.wantVersion)
			require.Equal(t, want, calls)
		})
	}
}

func Test_Delete_Kubernetes(t *testing.T) {
	// Note: unfortunately there isn't a great way to test a deletion failure with the runtime client.

	t.Run("success - lookup API Version (preferred namespaced resources)", func(t *testing.T) {
		client := fake.NewClientBuilder().WithObjects(&corev1.Secret{
			Name:      "test-name",
			Namespace: "test-namespace",
		}).Build()

		dc := &k8sutil.DiscoveryClient{
			Resources: []*metav1.APIResourceList{
				{
					GroupVersion: "v1",
					APIResources: []metav1.APIResource{
						{
							Name:    "api1",
							Version: "v1",
							Kind:    "Secret",
						},
					},
				},
			},
		}

		kcp := kubernetesclientprovider.FromConfig(nil)
		kcp.SetRuntimeClient(client)
		kcp.SetDiscoveryClient(dc)

		c := NewResourceClient(nil, nil, kcp)

		err := c.Delete(t.Context(), KubernetesCoreGroupResourceID)
		require.NoError(t, err)
	})

	t.Run("success - lookup API Version (preferred empty namespace)", func(t *testing.T) {
		client := fake.NewClientBuilder().WithObjects(&corev1.Secret{
			Name: "test-name",
		}).Build()

		dc := &k8sutil.DiscoveryClient{
			Resources: []*metav1.APIResourceList{
				{
					GroupVersion: "v1",
					APIResources: []metav1.APIResource{
						{
							Name:    "api1",
							Version: "v1",
							Kind:    "Secret",
						},
					},
				},
			},
		}

		kcp := kubernetesclientprovider.FromConfig(nil)
		kcp.SetRuntimeClient(client)
		kcp.SetDiscoveryClient(dc)

		c := NewResourceClient(nil, nil, kcp)

		err := c.Delete(t.Context(), KubernetesCoreGroupResourceID)
		require.NoError(t, err)
	})

	t.Run("failure - lookup API Version - resource list not found", func(t *testing.T) {
		client := fake.NewClientBuilder().WithObjects(&corev1.Secret{
			Name:      "test-name",
			Namespace: "test-namespace",
		}).Build()

		dc := &k8sutil.DiscoveryClient{
			Resources: []*metav1.APIResourceList{},
		}

		kcp := kubernetesclientprovider.FromConfig(nil)
		kcp.SetRuntimeClient(client)
		kcp.SetDiscoveryClient(dc)

		c := NewResourceClient(nil, nil, kcp)

		err := c.Delete(t.Context(), KubernetesCoreGroupResourceID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not find API version for type \"core/Secret\", type was not found")
	})
}

func Test_Delete_UCP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(AWSResourceID, handleDeleteSuccess())

		server := httptest.NewServer(mux)
		defer server.Close()

		connection, err := sdk.NewDirectConnection(server.URL)
		require.NoError(t, err)

		c := NewResourceClient(nil, connection, nil)

		err = c.Delete(t.Context(), AWSResourceID)
		require.NoError(t, err)
	})

	t.Run("success - delete returns 404", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(AWSResourceID, handleNotFound(t))

		server := httptest.NewServer(mux)
		defer server.Close()

		connection, err := sdk.NewDirectConnection(server.URL)
		require.NoError(t, err)

		c := NewResourceClient(nil, connection, nil)

		err = c.Delete(t.Context(), AWSResourceID)
		require.NoError(t, err)
	})

	t.Run("failure - delete fails", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(AWSResourceID, handleJSONResponse(t, v1.ErrorResponse{
			Error: &v1.ErrorDetails{
				Code: v1.CodeConflict,
			},
		}, 409))

		server := httptest.NewServer(mux)
		defer server.Close()

		connection, err := sdk.NewDirectConnection(server.URL)
		require.NoError(t, err)

		c := NewResourceClient(nil, connection, nil)

		err = c.Delete(t.Context(), AWSResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
	})
}

func Test_GetProperties(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(AWSResourceID, func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			handleJSONResponse(t, map[string]any{
				"id":         AWSResourceID,
				"name":       "test-stream",
				"type":       "AWS.Kinesis/Streams",
				"properties": map[string]any{"Arn": "arn:aws:kinesis:us-east-1:0000:stream/test-stream", "Name": "test-stream"},
			}, 200)(w, r)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		connection, err := sdk.NewDirectConnection(server.URL)
		require.NoError(t, err)

		c := NewResourceClient(nil, connection, nil)

		properties, err := c.GetProperties(t.Context(), AWSResourceID)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"Arn": "arn:aws:kinesis:us-east-1:0000:stream/test-stream", "Name": "test-stream"}, properties)
	})

	t.Run("failure - resource not found", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(AWSResourceID, handleNotFound(t))

		server := httptest.NewServer(mux)
		defer server.Close()

		connection, err := sdk.NewDirectConnection(server.URL)
		require.NoError(t, err)

		c := NewResourceClient(nil, connection, nil)

		properties, err := c.GetProperties(t.Context(), AWSResourceID)
		require.Nil(t, properties)
		require.IsType(t, &ResourceError{}, err)
		require.Contains(t, err.Error(), AWSResourceID)
	})

	t.Run("failure - not a UCP resource", func(t *testing.T) {
		c := NewResourceClient(nil, nil, nil)

		properties, err := c.GetProperties(t.Context(), "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/account")
		require.Nil(t, properties)
		require.EqualError(t, err, "resource \"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/account\" is not a UCP resource")
	})

	t.Run("failure - connection has no endpoint", func(t *testing.T) {
		c := NewResourceClient(nil, noEndpointConnection{}, nil)

		properties, err := c.GetProperties(t.Context(), AWSResourceID)
		require.Nil(t, properties)
		require.ErrorContains(t, err, "missing Azure Resource Manager configuration")
	})

	t.Run("failure - invalid resource ID", func(t *testing.T) {
		c := NewResourceClient(nil, nil, nil)

		properties, err := c.GetProperties(t.Context(), "invalid")
		require.Nil(t, properties)
		require.Error(t, err)
	})
}

func newArmOptions(url string) *armauth.ArmConfig {
	return &armauth.ArmConfig{
		ClientOptions: clientv2.Options{
			Cred:    &aztoken.AnonymousCredential{},
			BaseURI: url,
		},
	}
}

func newClientOptions(c *http.Client, url string) *arm.ClientOptions {
	return &arm.ClientOptions{
		Transport: &wrapper{Client: c},
		Cloud: cloud.Configuration{
			Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
				cloud.ResourceManager: {
					Endpoint: url,
					Audience: "https://management.core.windows.net",
				},
			},
		},
		// When updating azcore to 1.11.1 from 1.7.0, we saw that HTTPS check for Authentication was added.
		// Link to the check: https://github.com/Azure/azure-sdk-for-go/blob/main/sdk/azcore/runtime/policy_bearer_token.go#L118
		//
		// This check was failing for ARM requests over HTTP. To fix this, we set InsecureAllowCredentialWithHTTP to true.
		// The reason it was failing is because the ARM requests are made over HTTP and the bearer token is being sent in the header.
		InsecureAllowCredentialWithHTTP: true,
	}
}

func handleDeleteSuccess() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(204)
	}
}

func handleJSONResponse(t *testing.T, response any, statusCode int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		b, err := json.Marshal(&response)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, err = w.Write(b)
		require.NoError(t, err)
	}
}

func handleNotFound(t *testing.T) func(w http.ResponseWriter, r *http.Request) {
	return handleJSONResponse(t, v1.ErrorResponse{
		Error: &v1.ErrorDetails{
			Code: v1.CodeNotFound,
		},
	}, 404)
}

// wrapper implements the INTERNAL interface that autorest uses for transport :(.
type wrapper struct {
	Client *http.Client
}

// Do is a method that sends an HTTP request and returns an HTTP response and an error if one occurs.
func (w *wrapper) Do(req *http.Request) (*http.Response, error) {
	return w.Client.Do(req)
}

// noEndpointConnection is a UCP connection with no endpoint, which the generated UCP clients reject.
type noEndpointConnection struct{}

func (noEndpointConnection) Client() *http.Client { return http.DefaultClient }

func (noEndpointConnection) Endpoint() string { return "" }
