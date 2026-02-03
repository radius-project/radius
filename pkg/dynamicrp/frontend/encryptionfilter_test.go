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
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

const (
	testResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/myResource"
	testAPIVersion = "2023-10-01-preview"
)

func TestMakeEncryptionFilter_NilHandler(t *testing.T) {
	// When handler is nil, filter should pass through without error
	filter := makeEncryptionFilter(nil, nil)

	ctx := createTestContext()
	resource := &datamodel.DynamicResource{
		Properties: map[string]any{
			"password": "secret123",
		},
	}

	response, err := filter(ctx, resource, nil, nil)
	require.NoError(t, err)
	require.Nil(t, response)

	// Password should remain unchanged (not encrypted)
	require.Equal(t, "secret123", resource.Properties["password"])
}

func TestMakeEncryptionFilter_NoSensitiveFields(t *testing.T) {
	// When schema has no sensitive fields, data passes through unchanged
	ucpClient, err := testUCPClientFactoryNoSensitiveFields()
	require.NoError(t, err)

	handler := createTestHandler(t)
	filter := makeEncryptionFilter(ucpClient, handler)

	ctx := createTestContext()
	resource := &datamodel.DynamicResource{
		Properties: map[string]any{
			"name":  "test",
			"value": "not-sensitive",
		},
	}

	response, err := filter(ctx, resource, nil, nil)
	require.NoError(t, err)
	require.Nil(t, response)

	// Values should remain unchanged
	require.Equal(t, "test", resource.Properties["name"])
	require.Equal(t, "not-sensitive", resource.Properties["value"])
}

func TestMakeEncryptionFilter_WithSensitiveFields(t *testing.T) {
	// When schema has sensitive fields, they should be encrypted
	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	handler := createTestHandler(t)
	filter := makeEncryptionFilter(ucpClient, handler)

	ctx := createTestContext()
	resource := &datamodel.DynamicResource{
		Properties: map[string]any{
			"name":     "test",
			"password": "secret123",
		},
	}

	response, err := filter(ctx, resource, nil, nil)
	require.NoError(t, err)
	require.Nil(t, response)

	// Name should remain unchanged
	require.Equal(t, "test", resource.Properties["name"])

	// Password should be encrypted (transformed to a map with encrypted data)
	encryptedData, ok := resource.Properties["password"].(map[string]any)
	require.True(t, ok, "password should be encrypted to a map")
	require.Contains(t, encryptedData, "encrypted")
	require.Contains(t, encryptedData, "nonce")
	require.Contains(t, encryptedData, "version")
}

func TestMakeEncryptionFilter_NilProperties(t *testing.T) {
	// When resource has nil properties, filter should pass through
	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	handler := createTestHandler(t)
	filter := makeEncryptionFilter(ucpClient, handler)

	ctx := createTestContext()
	resource := &datamodel.DynamicResource{
		Properties: nil,
	}

	response, err := filter(ctx, resource, nil, nil)
	require.NoError(t, err)
	require.Nil(t, response)
}

func TestMakeEncryptionFilter_SchemaFetchError(t *testing.T) {
	// When schema fetch fails with an error, filter should return an error response
	ucpClient, err := testUCPClientFactoryWithError()
	require.NoError(t, err)

	handler := createTestHandler(t)
	filter := makeEncryptionFilter(ucpClient, handler)

	ctx := createTestContext()
	resource := &datamodel.DynamicResource{
		Properties: map[string]any{
			"password": "secret123",
		},
	}

	response, err := filter(ctx, resource, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, response, "expected error response when schema fetch fails")

	// Password should remain unchanged (encryption was not attempted due to error)
	require.Equal(t, "secret123", resource.Properties["password"])
}

func TestMakeEncryptionFilter_NestedSensitiveFields(t *testing.T) {
	// Test encryption of nested sensitive fields
	ucpClient, err := testUCPClientFactoryWithNestedSensitiveFields()
	require.NoError(t, err)

	handler := createTestHandler(t)
	filter := makeEncryptionFilter(ucpClient, handler)

	ctx := createTestContext()
	resource := &datamodel.DynamicResource{
		Properties: map[string]any{
			"name": "test",
			"credentials": map[string]any{
				"username": "user",
				"password": "secret123",
			},
		},
	}

	response, err := filter(ctx, resource, nil, nil)
	require.NoError(t, err)
	require.Nil(t, response)

	// Name should remain unchanged
	require.Equal(t, "test", resource.Properties["name"])

	// Credentials.password should be encrypted
	credentials, ok := resource.Properties["credentials"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", credentials["username"])

	encryptedPassword, ok := credentials["password"].(map[string]any)
	require.True(t, ok, "credentials.password should be encrypted to a map")
	require.Contains(t, encryptedPassword, "encrypted")
	require.Contains(t, encryptedPassword, "nonce")
}

// Helper functions

func createTestContext() context.Context {
	ctx := context.Background()
	// Add ARM request context
	armCtx := &v1.ARMRequestContext{
		ResourceID: mustParseResourceID(testResourceID),
		APIVersion: testAPIVersion,
	}
	return v1.WithARMRequestContext(ctx, armCtx)
}

func mustParseResourceID(id string) resources.ID {
	resourceID, err := resources.Parse(id)
	if err != nil {
		panic(err)
	}
	return resourceID
}

func createTestHandler(t *testing.T) *encryption.SensitiveDataHandler {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)

	provider, err := encryption.NewInMemoryKeyProvider(key)
	require.NoError(t, err)

	handler, err := encryption.NewSensitiveDataHandlerFromProvider(context.Background(), provider)
	require.NoError(t, err)

	return handler
}

func testUCPClientFactoryNoSensitiveFields() (*v20231001preview.ClientFactory, error) {
	return createFakeUCPClientFactory(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
			},
			"value": map[string]any{
				"type": "string",
			},
		},
	})
}

func testUCPClientFactoryWithSensitiveFields() (*v20231001preview.ClientFactory, error) {
	return createFakeUCPClientFactory(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
			},
			"password": map[string]any{
				"type":               "string",
				"x-radius-sensitive": true,
			},
		},
	})
}

func testUCPClientFactoryWithNestedSensitiveFields() (*v20231001preview.ClientFactory, error) {
	return createFakeUCPClientFactory(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
			},
			"credentials": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"username": map[string]any{
						"type": "string",
					},
					"password": map[string]any{
						"type":               "string",
						"x-radius-sensitive": true,
					},
				},
			},
		},
	})
}

func testUCPClientFactoryWithError() (*v20231001preview.ClientFactory, error) {
	apiVersionsServer := fake.APIVersionsServer{
		Get: func(ctx context.Context, planeName, resourceProviderName, resourceTypeName, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
			errResp.SetResponseError(http.StatusNotFound, "NotFound")
			return
		},
	}

	return v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: fake.NewAPIVersionsServerTransport(&apiVersionsServer),
		},
	})
}

func createFakeUCPClientFactory(schema map[string]any) (*v20231001preview.ClientFactory, error) {
	apiVersionsServer := fake.APIVersionsServer{
		Get: func(ctx context.Context, planeName, resourceProviderName, resourceTypeName, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.APIVersionsClientGetResponse{
				APIVersionResource: v20231001preview.APIVersionResource{
					Name: to.Ptr(apiVersionName),
					Properties: &v20231001preview.APIVersionProperties{
						Schema: schema,
					},
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}

	return v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: fake.NewAPIVersionsServerTransport(&apiVersionsServer),
		},
	})
}
