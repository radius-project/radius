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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/azure/armauth"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

func Test_Delete_InvalidResourceID(t *testing.T) {
	c := NewResourceClient(nil, nil, nil, nil)
	err := c.Delete(context.Background(), "invalid")
	require.Error(t, err)
}

func Test_Delete_ARM(t *testing.T) {
	t.Run("failure - delete fails", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMResourceID, handleJSONResponse(t, v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code: v1.CodeConflict,
			},
		}, 409))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(context.Background(), ARMResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
	})

	t.Run("success - lookup API Version (default)", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMResourceID, handleDeleteSuccess(t))
		mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace: to.Ptr("Microsoft.Compute"),
			ResourceTypes: []*armresources.ProviderResourceType{
				{
					ResourceType: to.Ptr("anotherType"),
					APIVersions:  []*string{},
				},
				{
					ResourceType:      to.Ptr("virtualMachines"),
					DefaultAPIVersion: to.Ptr(ARMAPIVersion),
				},
			},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(context.Background(), ARMResourceID)
		require.NoError(t, err)
	})

	t.Run("success - lookup API Version (first available)", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMResourceID, handleDeleteSuccess(t))
		mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace: to.Ptr("Microsoft.Compute"),
			ResourceTypes: []*armresources.ProviderResourceType{
				{
					ResourceType: to.Ptr("virtualMachines"),
					APIVersions:  []*string{to.Ptr("2020-01-01"), to.Ptr("2020-01-02")},
				},
			},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(context.Background(), ARMResourceID)
		require.NoError(t, err)
	})

	t.Run("failure - lookup API Version - provider not found", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMProviderPath, handleNotFound(t))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(context.Background(), ARMResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
	})

	t.Run("failure - lookup API Version - resource type not found", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace:     to.Ptr("Microsoft.Compute"),
			ResourceTypes: []*armresources.ProviderResourceType{},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(context.Background(), ARMResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
		require.Contains(t, err.Error(), "could not find API version for type \"Microsoft.Compute/virtualMachines\", type was not found")
	})

	t.Run("failure - lookup API Version - no api versions", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
			Namespace: to.Ptr("Microsoft.Compute"),
			ResourceTypes: []*armresources.ProviderResourceType{
				{
					ResourceType: to.Ptr("virtualMachines"),
					APIVersions:  []*string{},
				},
			},
		}, 200))

		server := httptest.NewServer(mux)
		defer server.Close()

		c := NewResourceClient(newArmOptions(server.URL), nil, nil, nil)
		c.armClientOptions = newClientOptions(server.Client(), server.URL)

		err := c.Delete(context.Background(), ARMResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
		require.Contains(t, err.Error(), "could not find API version for type \"Microsoft.Compute/virtualMachines\", no supported API versions")
	})
}

func Test_Delete_Kubernetes(t *testing.T) {
	// Note: unfortunately there isn't a great way to test a deletion failure with the runtime client.

	t.Run("success - lookup API Version (preferred namespaced resources)", func(t *testing.T) {
		client := fake.NewClientBuilder().WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-namespace",
			},
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

		c := NewResourceClient(nil, nil, client, dc)

		err := c.Delete(context.Background(), KubernetesCoreGroupResourceID)
		require.NoError(t, err)
	})

	t.Run("success - lookup API Version (preferred empty namespace)", func(t *testing.T) {
		client := fake.NewClientBuilder().WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-name",
			},
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

		c := NewResourceClient(nil, nil, client, dc)

		err := c.Delete(context.Background(), KubernetesCoreGroupResourceID)
		require.NoError(t, err)
	})

	t.Run("failure - lookup API Version - resource list not found", func(t *testing.T) {
		client := fake.NewClientBuilder().WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-name",
				Namespace: "test-namespace",
			},
		}).Build()

		dc := &k8sutil.DiscoveryClient{
			Resources: []*metav1.APIResourceList{},
		}

		c := NewResourceClient(nil, nil, client, dc)

		err := c.Delete(context.Background(), KubernetesCoreGroupResourceID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not find API version for type \"core/Secret\", type was not found")
	})
}

func Test_Delete_UCP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(AWSResourceID, handleDeleteSuccess(t))

		server := httptest.NewServer(mux)
		defer server.Close()

		connection, err := sdk.NewDirectConnection(server.URL)
		require.NoError(t, err)

		c := NewResourceClient(nil, connection, nil, nil)

		err = c.Delete(context.Background(), AWSResourceID)
		require.NoError(t, err)
	})

	t.Run("failure - delete fails", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc(AWSResourceID, handleJSONResponse(t, v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code: v1.CodeConflict,
			},
		}, 409))

		server := httptest.NewServer(mux)
		defer server.Close()

		connection, err := sdk.NewDirectConnection(server.URL)
		require.NoError(t, err)

		c := NewResourceClient(nil, connection, nil, nil)

		err = c.Delete(context.Background(), AWSResourceID)
		require.Error(t, err)
		require.IsType(t, &ResourceError{}, err)
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
		ClientOptions: policy.ClientOptions{
			Transport: &wrapper{Client: c},
			Cloud: cloud.Configuration{
				Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
					cloud.ResourceManager: {
						Endpoint: url,
						Audience: "https://management.core.windows.net",
					},
				},
			},
		},
	}
}

func handleDeleteSuccess(t *testing.T) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
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
		Error: v1.ErrorDetails{
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
