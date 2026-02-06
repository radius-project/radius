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

package frontend

import (
	"context"
	"net/http"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/stretchr/testify/require"
)

func TestPrepareResourceFilter(t *testing.T) {
	ctx := context.Background()

	t.Run("succeeds with sensitive fields", func(t *testing.T) {
		clientFactory, err := testUCPClientFactory(map[string]any{
			"properties": map[string]any{
				"host": map[string]any{
					"type": "string",
				},
				"password": map[string]any{
					"type":               "string",
					"x-radius-sensitive": true,
				},
			},
		})
		require.NoError(t, err)

		factory := &UpdateFilterFactory{
			UCPClient: clientFactory,
		}

		filter := factory.NewPrepareResourceFilter()

		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test",
					Type: "Foo.Bar/myResources",
				},
				InternalMetadata: v1.InternalMetadata{
					UpdatedAPIVersion: "2024-01-01",
				},
			},
			Properties: map[string]any{},
		}

		resp, err := filter(ctx, resource, nil, &controller.Options{})

		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("succeeds with no sensitive fields", func(t *testing.T) {
		clientFactory, err := testUCPClientFactory(map[string]any{
			"properties": map[string]any{
				"host": map[string]any{
					"type": "string",
				},
			},
		})
		require.NoError(t, err)

		factory := &UpdateFilterFactory{
			UCPClient: clientFactory,
		}

		filter := factory.NewPrepareResourceFilter()

		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test",
					Type: "Foo.Bar/myResources",
				},
				InternalMetadata: v1.InternalMetadata{
					UpdatedAPIVersion: "2024-01-01",
				},
			},
			Properties: map[string]any{},
		}

		resp, err := filter(ctx, resource, nil, &controller.Options{})

		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("succeeds with nil UCP client", func(t *testing.T) {
		factory := &UpdateFilterFactory{
			UCPClient: nil,
		}

		filter := factory.NewPrepareResourceFilter()

		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test",
					Type: "Foo.Bar/myResources",
				},
				InternalMetadata: v1.InternalMetadata{
					UpdatedAPIVersion: "2024-01-01",
				},
			},
			Properties: map[string]any{},
		}

		resp, err := filter(ctx, resource, nil, &controller.Options{})

		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("continues on schema fetch error", func(t *testing.T) {
		// Create a client that will fail to fetch the schema
		clientFactory, err := testUpdateFilterUCPClientFactoryWithError()
		require.NoError(t, err)

		factory := &UpdateFilterFactory{
			UCPClient: clientFactory,
		}

		filter := factory.NewPrepareResourceFilter()

		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test/providers/Foo.Bar/myResources/test",
					Type: "Foo.Bar/myResources",
				},
				InternalMetadata: v1.InternalMetadata{
					UpdatedAPIVersion: "2024-01-01",
				},
			},
			Properties: map[string]any{},
		}

		// Should not fail even if schema fetch fails
		resp, err := filter(ctx, resource, nil, &controller.Options{})

		require.NoError(t, err)
		require.Nil(t, resp)
	})
}

// testUCPClientFactory creates a mock UCP client factory that returns the given schema.
func testUCPClientFactory(schema map[string]any) (*v20231001preview.ClientFactory, error) {
	apiVersionsServer := fake.APIVersionsServer{
		Get: func(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (azfake.Responder[v20231001preview.APIVersionsClientGetResponse], azfake.ErrorResponder) {
			resp := azfake.Responder[v20231001preview.APIVersionsClientGetResponse]{}
			resp.SetResponse(http.StatusOK, v20231001preview.APIVersionsClientGetResponse{
				APIVersionResource: v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{
						Schema: schema,
					},
				},
			}, nil)
			return resp, azfake.ErrorResponder{}
		},
	}

	return v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
				APIVersionsServer: apiVersionsServer,
			}),
		},
	})
}

// testUpdateFilterUCPClientFactoryWithError creates a mock UCP client factory that returns an error.
func testUpdateFilterUCPClientFactoryWithError() (*v20231001preview.ClientFactory, error) {
	apiVersionsServer := fake.APIVersionsServer{
		Get: func(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (azfake.Responder[v20231001preview.APIVersionsClientGetResponse], azfake.ErrorResponder) {
			resp := azfake.Responder[v20231001preview.APIVersionsClientGetResponse]{}
			errResp := azfake.ErrorResponder{}
			errResp.SetResponseError(http.StatusNotFound, "NotFound")
			return resp, errResp
		},
	}

	return v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
				APIVersionsServer: apiVersionsServer,
			}),
		},
	})
}
