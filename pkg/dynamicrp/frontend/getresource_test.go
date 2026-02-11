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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testGetURL       = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/myResource?api-version=2023-10-01-preview"
	getTestResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/myResource"
	getTestAPIVersion = "2023-10-01-preview"
)

func newTestGetController(t *testing.T, databaseClient database.Client, ucpClient *v20231001preview.ClientFactory) controller.Controller {
	t.Helper()

	opts := controller.Options{
		DatabaseClient: databaseClient,
	}
	resourceOpts := controller.ResourceOptions[datamodel.DynamicResource]{
		ResponseConverter: converter.DynamicResourceDataModelToVersioned,
	}

	c, err := NewGetResourceWithRedaction(opts, resourceOpts, ucpClient)
	require.NoError(t, err)

	return c
}

func newGetTestDynamicResource(provisioningState v1.ProvisioningState, properties map[string]any) *datamodel.DynamicResource {
	return &datamodel.DynamicResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   getTestResourceID,
				Name: "myResource",
				Type: "Applications.Test/testResources",
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      getTestAPIVersion,
				AsyncProvisioningState: provisioningState,
			},
		},
		Properties: properties,
	}
}

func TestGetResourceWithRedaction_NonSucceededRedacts(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newGetTestDynamicResource(v1.ProvisioningStateAccepted, map[string]any{
		"password": "secret123",
	})

	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: getTestResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Get(gomock.Any(), getTestResourceID).
		Return(storeObject, nil)

	ucpClient, err := testGetUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	c := newTestGetController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testGetURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	_, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	_ = resp.Apply(ctx, w, req)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	properties, ok := body["properties"].(map[string]any)
	require.True(t, ok)
	require.Nil(t, properties["password"])
}

func TestGetResourceWithRedaction_SucceededSkipsRedaction(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newGetTestDynamicResource(v1.ProvisioningStateSucceeded, map[string]any{
		"password": "secret123",
	})

	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: getTestResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Get(gomock.Any(), getTestResourceID).
		Return(storeObject, nil)

	ucpClient, err := testGetUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	c := newTestGetController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testGetURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	_ = resp.Apply(ctx, w, req)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	properties, ok := body["properties"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "secret123", properties["password"])
}

func TestGetResourceWithRedaction_SchemaFetchErrorContinues(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newGetTestDynamicResource(v1.ProvisioningStateAccepted, map[string]any{
		"password": "secret123",
	})

	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: getTestResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Get(gomock.Any(), getTestResourceID).
		Return(storeObject, nil)

	ucpClient, err := testGetUCPClientFactoryWithError()
	require.NoError(t, err)

	c := newTestGetController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testGetURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	_ = resp.Apply(ctx, w, req)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	properties, ok := body["properties"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "secret123", properties["password"])
}

func testGetUCPClientFactoryWithSensitiveFields() (*v20231001preview.ClientFactory, error) {
	return createGetFakeUCPClientFactory(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"password": map[string]any{
				"type":               "string",
				"x-radius-sensitive": true,
			},
		},
	})
}

func testGetUCPClientFactoryWithError() (*v20231001preview.ClientFactory, error) {
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

func createGetFakeUCPClientFactory(schema map[string]any) (*v20231001preview.ClientFactory, error) {
	apiVersionsServer := fake.APIVersionsServer{
		Get: func(ctx context.Context, planeName, resourceProviderName, resourceTypeName, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.APIVersionsClientGetResponse{
				APIVersionResource: v20231001preview.APIVersionResource{
					Name: &apiVersionName,
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
}/*
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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactField_SimpleField(t *testing.T) {
	// Test redacting a simple top-level field
	properties := map[string]any{
		"name":     "test-resource",
		"password": "secret123",
		"data":     map[string]any{"key": "value"},
	}

	redactField(properties, "password")

	require.Equal(t, "test-resource", properties["name"])
	require.Nil(t, properties["password"])
	require.NotNil(t, properties["data"])
}

func TestRedactField_DataField(t *testing.T) {
	// Test redacting the "data" field (common pattern for secrets)
	properties := map[string]any{
		"environment": "/planes/radius/local/resourcegroups/default/providers/Radius.Core/environments/test",
		"application": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/applications/test",
		"data": map[string]any{
			"password": map[string]any{
				"value":    "secret123",
				"encoding": "string",
			},
			"apiKey": map[string]any{
				"value": "my-api-key",
			},
		},
	}

	redactField(properties, "data")

	require.NotNil(t, properties["environment"])
	require.NotNil(t, properties["application"])
	require.Nil(t, properties["data"])
}

func TestRedactField_NonExistentField(t *testing.T) {
	// Test that redacting a non-existent field doesn't cause errors
	properties := map[string]any{
		"name":  "test-resource",
		"value": "test-value",
	}

	// Should not panic or error
	redactField(properties, "nonexistent")

	// Original fields should remain unchanged
	require.Equal(t, "test-resource", properties["name"])
	require.Equal(t, "test-value", properties["value"])
}

func TestRedactField_NilProperties(t *testing.T) {
	// Test that nil properties don't cause panic
	var properties map[string]any

	// Should not panic
	redactField(properties, "anyfield")
}

func TestRedactField_EmptyProperties(t *testing.T) {
	// Test redacting from empty properties
	properties := map[string]any{}

	redactField(properties, "password")

	require.Empty(t, properties)
}

func TestRedactField_MultipleFields(t *testing.T) {
	// Test redacting multiple fields sequentially
	properties := map[string]any{
		"name":     "test",
		"password": "secret",
		"apiKey":   "key123",
		"data":     "sensitive-data",
	}

	redactField(properties, "password")
	redactField(properties, "apiKey")
	redactField(properties, "data")

	require.Equal(t, "test", properties["name"])
	require.Nil(t, properties["password"])
	require.Nil(t, properties["apiKey"])
	require.Nil(t, properties["data"])
}

func TestRedactField_NestedDotPath(t *testing.T) {
	// Test redacting a nested field via dot-separated path
	properties := map[string]any{
		"config": map[string]any{
			"password": "secret",
			"host":     "localhost",
		},
	}

	redactField(properties, "config.password")

	config, ok := properties["config"].(map[string]any)
	require.True(t, ok)
	require.Nil(t, config["password"])
	require.Equal(t, "localhost", config["host"])
}

func TestRedactField_DeeplyNestedDotPath(t *testing.T) {
	// Test redacting a deeply nested field
	properties := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"secret": "top-secret",
				"other":  "keep-this",
			},
		},
	}

	redactField(properties, "level1.level2.secret")

	level1 := properties["level1"].(map[string]any)
	level2 := level1["level2"].(map[string]any)
	require.Nil(t, level2["secret"])
	require.Equal(t, "keep-this", level2["other"])
}

func TestRedactField_ArrayWildcard(t *testing.T) {
	// Test redacting fields within array elements using [*]
	properties := map[string]any{
		"secrets": []any{
			map[string]any{"name": "secret1", "value": "s1"},
			map[string]any{"name": "secret2", "value": "s2"},
		},
	}

	redactField(properties, "secrets[*].value")

	secrets := properties["secrets"].([]any)
	s0 := secrets[0].(map[string]any)
	s1 := secrets[1].(map[string]any)
	require.Nil(t, s0["value"])
	require.Nil(t, s1["value"])
	require.Equal(t, "secret1", s0["name"])
	require.Equal(t, "secret2", s1["name"])
}

func TestRedactField_MapWildcard(t *testing.T) {
	// Test redacting all values of a map using [*]
	properties := map[string]any{
		"config": map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}

	redactField(properties, "config[*]")

	config := properties["config"].(map[string]any)
	require.Nil(t, config["key1"])
	require.Nil(t, config["key2"])
}

func TestRedactField_MapWildcardWithNestedField(t *testing.T) {
	// Test redacting a nested field within map values using [*]
	properties := map[string]any{
		"backends": map[string]any{
			"kv":    map[string]any{"url": "http://vault", "token": "secret-token"},
			"azure": map[string]any{"url": "http://azure", "token": "azure-token"},
		},
	}

	redactField(properties, "backends[*].token")

	backends := properties["backends"].(map[string]any)
	kv := backends["kv"].(map[string]any)
	azure := backends["azure"].(map[string]any)
	require.Nil(t, kv["token"])
	require.Nil(t, azure["token"])
	require.Equal(t, "http://vault", kv["url"])
	require.Equal(t, "http://azure", azure["url"])
}

func TestRedactField_NestedPathFieldNotFound(t *testing.T) {
	// Test that a non-existent nested path doesn't cause errors
	properties := map[string]any{
		"config": map[string]any{
			"host": "localhost",
		},
	}

	// Should not panic - field doesn't exist at this nested path
	redactField(properties, "config.nonexistent")

	config := properties["config"].(map[string]any)
	require.Equal(t, "localhost", config["host"])
}

func TestRedactField_EmptyPath(t *testing.T) {
	// Test that empty path doesn't cause errors
	properties := map[string]any{
		"data": "value",
	}

	redactField(properties, "")

	require.Equal(t, "value", properties["data"])
}

func TestRedactField_ArrayWildcardAllElements(t *testing.T) {
	// Test redacting all elements in an array using [*] as the final segment
	properties := map[string]any{
		"tokens": []any{"token1", "token2", "token3"},
	}

	redactField(properties, "tokens[*]")

	tokens := properties["tokens"].([]any)
	for _, token := range tokens {
		require.Nil(t, token)
	}
}

func TestRedactField_FieldWithNilValue(t *testing.T) {
	// Test redacting a field that already has nil value
	properties := map[string]any{
		"name":     "test",
		"password": nil,
	}

	redactField(properties, "password")

	require.Equal(t, "test", properties["name"])
	require.Nil(t, properties["password"])
}

func TestRedactField_FieldWithDifferentTypes(t *testing.T) {
	// Test redacting fields of various types
	testCases := []struct {
		name       string
		value      any
		fieldName  string
		properties map[string]any
	}{
		{
			name:       "string field",
			fieldName:  "secret",
			properties: map[string]any{"secret": "password123"},
		},
		{
			name:       "map field",
			fieldName:  "data",
			properties: map[string]any{"data": map[string]any{"key": "value"}},
		},
		{
			name:       "slice field",
			fieldName:  "tokens",
			properties: map[string]any{"tokens": []string{"token1", "token2"}},
		},
		{
			name:       "int field",
			fieldName:  "pin",
			properties: map[string]any{"pin": 1234},
		},
		{
			name:       "bool field",
			fieldName:  "enabled",
			properties: map[string]any{"enabled": true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			redactField(tc.properties, tc.fieldName)
			require.Nil(t, tc.properties[tc.fieldName])
		})
	}
}

func TestParseRedactPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []redactPathSegment
	}{
		{
			name:     "simple field",
			path:     "data",
			expected: []redactPathSegment{{name: "data"}},
		},
		{
			name:     "nested dot path",
			path:     "credentials.password",
			expected: []redactPathSegment{{name: "credentials"}, {name: "password"}},
		},
		{
			name:     "array wildcard",
			path:     "secrets[*].value",
			expected: []redactPathSegment{{name: "secrets"}, {wildcard: true}, {name: "value"}},
		},
		{
			name:     "map wildcard",
			path:     "config[*]",
			expected: []redactPathSegment{{name: "config"}, {wildcard: true}},
		},
		{
			name:     "deeply nested",
			path:     "a.b.c.d",
			expected: []redactPathSegment{{name: "a"}, {name: "b"}, {name: "c"}, {name: "d"}},
		},
		{
			name:     "wildcard with nested field",
			path:     "backends[*].token",
			expected: []redactPathSegment{{name: "backends"}, {wildcard: true}, {name: "token"}},
		},
		{
			name:     "empty path",
			path:     "",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := parseRedactPath(tc.path)
			require.Equal(t, tc.expected, result)
		})
	}
}
