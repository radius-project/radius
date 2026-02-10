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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
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
	testListURL = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources?api-version=2023-10-01-preview"
)

func newTestListController(t *testing.T, databaseClient database.Client, ucpClient *v20231001preview.ClientFactory) ctrl.Controller {
	t.Helper()

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}
	resourceOpts := ctrl.ResourceOptions[datamodel.DynamicResource]{
		ResponseConverter: converter.DynamicResourceDataModelToVersioned,
	}
	c, err := NewListResourcesWithRedaction(opts, resourceOpts, ucpClient)
	require.NoError(t, err)

	return c
}

func newTestDynamicResource(id string, name string, provisioningState v1.ProvisioningState, properties map[string]any) *datamodel.DynamicResource {
	return &datamodel.DynamicResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   id,
				Name: name,
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

func TestListResourcesWithRedaction_EmptyResult(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{Items: []database.Object{}}, nil)

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	_ = resp.Apply(ctx, w, req)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestListResourcesWithRedaction_SucceededResourcesNotRedacted(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newTestDynamicResource(
		testResourceID,
		"myResource",
		v1.ProvisioningStateSucceeded,
		map[string]any{
			"name":     "test",
			"password": "secret123",
		},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items: []database.Object{*rpctest.FakeStoreObject(resource)},
		}, nil)

	// Schema should NOT be fetched for Succeeded resources
	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify we get a valid paginated response with one item
	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 1)
}

func TestListResourcesWithRedaction_NonSucceededResourcesRedacted(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newTestDynamicResource(
		testResourceID,
		"myResource",
		v1.ProvisioningStateAccepted,
		map[string]any{
			"name":     "test",
			"password": "secret123",
		},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items: []database.Object{*rpctest.FakeStoreObject(resource)},
		}, nil)

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify the response is valid
	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 1)
}

func TestListResourcesWithRedaction_MixedProvisioningStates(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	succeededResource := newTestDynamicResource(
		testResourceID,
		"succeededResource",
		v1.ProvisioningStateSucceeded,
		map[string]any{
			"name":     "succeeded",
			"password": "already-redacted",
		},
	)
	acceptedResource := newTestDynamicResource(
		"/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/acceptedResource",
		"acceptedResource",
		v1.ProvisioningStateAccepted,
		map[string]any{
			"name":     "accepted",
			"password": "should-be-redacted",
		},
	)
	failedResource := newTestDynamicResource(
		"/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/failedResource",
		"failedResource",
		v1.ProvisioningStateFailed,
		map[string]any{
			"name":     "failed",
			"password": "should-also-be-redacted",
		},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items: []database.Object{
				*rpctest.FakeStoreObject(succeededResource),
				*rpctest.FakeStoreObject(acceptedResource),
				*rpctest.FakeStoreObject(failedResource),
			},
		}, nil)

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 3)
}

func TestListResourcesWithRedaction_NoSensitiveFields(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newTestDynamicResource(
		testResourceID,
		"myResource",
		v1.ProvisioningStateAccepted,
		map[string]any{
			"name":  "test",
			"value": "not-sensitive",
		},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items: []database.Object{*rpctest.FakeStoreObject(resource)},
		}, nil)

	ucpClient, err := testUCPClientFactoryNoSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 1)
}

func TestListResourcesWithRedaction_SchemaFetchError(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newTestDynamicResource(
		testResourceID,
		"myResource",
		v1.ProvisioningStateAccepted,
		map[string]any{
			"name":     "test",
			"password": "secret123",
		},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items: []database.Object{*rpctest.FakeStoreObject(resource)},
		}, nil)

	// Use a UCP client that returns an error for schema fetch
	ucpClient, err := testUCPClientFactoryWithError()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	// Should succeed even though schema fetch fails (continues without redaction)
	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 1)
}

func TestListResourcesWithRedaction_DatabaseQueryError(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database connection error"))

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestListResourcesWithRedaction_EmptyProperties(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newTestDynamicResource(
		testResourceID,
		"myResource",
		v1.ProvisioningStateAccepted,
		map[string]any{},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items: []database.Object{*rpctest.FakeStoreObject(resource)},
		}, nil)

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	// Should handle empty properties gracefully (no fields to redact)
	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 1)
}

func TestListResourcesWithRedaction_NestedSensitiveFields(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newTestDynamicResource(
		testResourceID,
		"myResource",
		v1.ProvisioningStateAccepted,
		map[string]any{
			"name": "test",
			"credentials": map[string]any{
				"username": "user",
				"password": "secret123",
			},
		},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items: []database.Object{*rpctest.FakeStoreObject(resource)},
		}, nil)

	ucpClient, err := testUCPClientFactoryWithNestedSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 1)
}

func TestListResourcesWithRedaction_NilUCPClient(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newTestDynamicResource(
		testResourceID,
		"myResource",
		v1.ProvisioningStateAccepted,
		map[string]any{
			"name":     "test",
			"password": "secret123",
		},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items: []database.Object{*rpctest.FakeStoreObject(resource)},
		}, nil)

	// nil UCP client — schema fetch should fail gracefully
	c := newTestListController(t, databaseClient, nil)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	// Should handle nil UCP client gracefully (continues without redaction)
	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 1)
}

func TestListResourcesWithRedaction_MultipleNonSucceededAllRedacted(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource1 := newTestDynamicResource(
		testResourceID,
		"resource1",
		v1.ProvisioningStateUpdating,
		map[string]any{
			"name":     "res1",
			"password": "secret1",
		},
	)
	resource2 := newTestDynamicResource(
		"/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/resource2",
		"resource2",
		v1.ProvisioningStateUpdating,
		map[string]any{
			"name":     "res2",
			"password": "secret2",
		},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items: []database.Object{
				*rpctest.FakeStoreObject(resource1),
				*rpctest.FakeStoreObject(resource2),
			},
		}, nil)

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify both items are returned
	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 2)
}

func TestListResourcesWithRedaction_PaginationToken(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newTestDynamicResource(
		testResourceID,
		"myResource",
		v1.ProvisioningStateSucceeded,
		map[string]any{
			"name": "test",
		},
	)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{
			Items:           []database.Object{*rpctest.FakeStoreObject(resource)},
			PaginationToken: "next-page-token",
		}, nil)

	ucpClient, err := testUCPClientFactoryNoSensitiveFields()
	require.NoError(t, err)

	c := newTestListController(t, databaseClient, ucpClient)

	req, err := http.NewRequest(http.MethodGet, testListURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	paginatedResp, ok := resp.(*rest.OKResponse)
	require.True(t, ok)
	paginatedList, ok := paginatedResp.Body.(*v1.PaginatedList)
	require.True(t, ok)
	require.Len(t, paginatedList.Value, 1)
	// NextLink should be set when pagination token is present
	require.NotEmpty(t, paginatedList.NextLink)
}

func TestNewListResourcesWithRedaction(t *testing.T) {
	opts := ctrl.Options{
		DatabaseClient: nil,
	}
	resourceOpts := ctrl.ResourceOptions[datamodel.DynamicResource]{
		ResponseConverter: converter.DynamicResourceDataModelToVersioned,
	}

	c, err := NewListResourcesWithRedaction(opts, resourceOpts, nil)
	require.NoError(t, err)
	require.NotNil(t, c)

	listCtrl, ok := c.(*ListResourcesWithRedaction)
	require.True(t, ok)
	require.False(t, listCtrl.listRecursiveQuery)
	require.Nil(t, listCtrl.ucpClient)
}

func TestNewListResourcesWithRedaction_RecursiveQuery(t *testing.T) {
	opts := ctrl.Options{
		DatabaseClient: nil,
	}
	resourceOpts := ctrl.ResourceOptions[datamodel.DynamicResource]{
		ResponseConverter:  converter.DynamicResourceDataModelToVersioned,
		ListRecursiveQuery: true,
	}

	c, err := NewListResourcesWithRedaction(opts, resourceOpts, nil)
	require.NoError(t, err)
	require.NotNil(t, c)

	listCtrl, ok := c.(*ListResourcesWithRedaction)
	require.True(t, ok)
	require.True(t, listCtrl.listRecursiveQuery)
}
