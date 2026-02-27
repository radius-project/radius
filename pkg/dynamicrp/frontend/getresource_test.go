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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testGetURL = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/myResource?api-version=2023-10-01-preview"
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
				ID:   testResourceID,
				Name: "myResource",
				Type: "Applications.Test/testResources",
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      testAPIVersion,
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
	storeObject.Metadata = database.Metadata{ID: testResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Get(gomock.Any(), testResourceID).
		Return(storeObject, nil)

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
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
	storeObject.Metadata = database.Metadata{ID: testResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Get(gomock.Any(), testResourceID).
		Return(storeObject, nil)

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
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

func TestGetResourceWithRedaction_EmptyPropertiesNonSucceeded(t *testing.T) {
	// When Properties are empty and state is non-Succeeded, redaction should be
	// attempted but find nothing to redact, and the resource returned as 200 OK.
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newGetTestDynamicResource(v1.ProvisioningStateAccepted, map[string]any{})

	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: testResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Get(gomock.Any(), testResourceID).
		Return(storeObject, nil)

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
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
}

func TestGetResourceWithRedaction_SchemaFetchErrorReturnsError(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newGetTestDynamicResource(v1.ProvisioningStateAccepted, map[string]any{
		"password": "secret123",
	})

	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: testResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Get(gomock.Any(), testResourceID).
		Return(storeObject, nil)

	ucpClient, err := testUCPClientFactoryWithError()
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
	// Expect fail-safe behavior: return error instead of exposing potentially sensitive data
	require.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)

	var body v1.ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	require.Equal(t, v1.CodeInternal, body.Error.Code)
	require.Contains(t, body.Error.Message, "Failed to fetch schema for security validation")
}
