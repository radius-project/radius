// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler"
	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler/planes"
	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler/resourcegroups"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

const baseURI = "/planes"

func initializeTestEnv(t *testing.T, ucp ucphandler.UCPHandler, dbClient store.StorageClient) http.Handler {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	options := ServiceOptions{
		DBClient: dbClient,
		UcpHandler: ucphandler.UCPHandler{
			Planes:         ucp.Planes,
			ResourceGroups: ucp.ResourceGroups,
		},
	}
	service := NewService(options)
	s, err := service.Initialize(ctx)
	require.NoError(t, err)
	server := httptest.NewServer(s.Handler)
	t.Cleanup(server.Close)
	return server.Config.Handler
}

func requireJSON(t *testing.T, expected interface{}, w *httptest.ResponseRecorder) {
	bytes, err := json.Marshal(expected)
	require.NoError(t, err)
	require.JSONEq(t, string(bytes), w.Body.String())
}

// TODO: Once https://github.com/project-radius/radius/issues/2303 is fixed,
// add more tests with trailing slash in the request URI

func Test_Handler_CreateOrUpdatePlane(t *testing.T) {
	ctrl := gomock.NewController(t)
	planesUCP := planes.NewMockPlanesUCPHandler(ctrl)
	ucp := ucphandler.UCPHandler{
		Planes:         planesUCP,
		ResourceGroups: resourcegroups.NewMockResourceGroupsUCPHandler(ctrl),
	}
	dbClient := store.NewMockStorageClient(ctrl)
	handler := initializeTestEnv(t, ucp, dbClient)

	planesUCP.EXPECT().CreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, DB store.StorageClient, body []byte, path string) (rest.Response, error) {
		return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
	})

	requestBody := map[string]interface{}{
		"tags": map[string]interface{}{
			"test-tag": "test-value",
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("PUT", baseURI+"/radius/local", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	requireJSON(t, map[string]interface{}{}, w)
}

func Test_Handler_GetPlane(t *testing.T) {
	ctrl := gomock.NewController(t)
	planesUCP := planes.NewMockPlanesUCPHandler(ctrl)
	ucp := ucphandler.UCPHandler{
		Planes:         planesUCP,
		ResourceGroups: resourcegroups.NewMockResourceGroupsUCPHandler(ctrl),
	}
	dbClient := store.NewMockStorageClient(ctrl)
	handler := initializeTestEnv(t, ucp, dbClient)

	planesUCP.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, DB store.StorageClient, path string) (rest.Response, error) {
		return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
	})
	req := httptest.NewRequest("GET", baseURI+"/radius/local", bytes.NewBuffer([]byte{}))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	requireJSON(t, map[string]interface{}{}, w)
}

func Test_Handler_ListAllPlanes(t *testing.T) {
	ctrl := gomock.NewController(t)
	planesUCP := planes.NewMockPlanesUCPHandler(ctrl)
	ucp := ucphandler.UCPHandler{
		Planes:         planesUCP,
		ResourceGroups: resourcegroups.NewMockResourceGroupsUCPHandler(ctrl),
	}
	dbClient := store.NewMockStorageClient(ctrl)
	handler := initializeTestEnv(t, ucp, dbClient)

	planesUCP.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, DB store.StorageClient, path string) (rest.Response, error) {
		return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
	})

	req := httptest.NewRequest("GET", baseURI, bytes.NewBuffer([]byte{}))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	requireJSON(t, map[string]interface{}{}, w)
}

func Test_Handler_DeletePlane(t *testing.T) {
	ctrl := gomock.NewController(t)
	planesUCP := planes.NewMockPlanesUCPHandler(ctrl)
	ucp := ucphandler.UCPHandler{
		Planes:         planesUCP,
		ResourceGroups: resourcegroups.NewMockResourceGroupsUCPHandler(ctrl),
	}
	dbClient := store.NewMockStorageClient(ctrl)
	handler := initializeTestEnv(t, ucp, dbClient)

	planesUCP.EXPECT().DeleteByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, DB store.StorageClient, path string) (rest.Response, error) {
		return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
	})

	req := httptest.NewRequest("DELETE", baseURI+"/radius/local", bytes.NewBuffer([]byte{}))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	requireJSON(t, map[string]interface{}{}, w)
}

func Test_Handler_ProxyPlaneRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	planesUCP := planes.NewMockPlanesUCPHandler(ctrl)
	ucp := ucphandler.UCPHandler{
		Planes:         planesUCP,
		ResourceGroups: resourcegroups.NewMockResourceGroupsUCPHandler(ctrl),
	}
	dbClient := store.NewMockStorageClient(ctrl)
	handler := initializeTestEnv(t, ucp, dbClient)

	planesUCP.EXPECT().ProxyRequest(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, db store.StorageClient, w http.ResponseWriter, r *http.Request, path string) (rest.Response, error) {
		return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
	})

	requestBody := map[string]interface{}{
		"tags": map[string]interface{}{
			"test-tag": "test-value",
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("PUT", baseURI+"/radius/local/foo", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
}

func Test_Handler_CreateResourceGroup(t *testing.T) {
	ctrl := gomock.NewController(t)
	rgUCP := resourcegroups.NewMockResourceGroupsUCPHandler(ctrl)
	dbClient := store.NewMockStorageClient(ctrl)
	ucp := ucphandler.UCPHandler{
		Planes:         planes.NewMockPlanesUCPHandler(ctrl),
		ResourceGroups: rgUCP,
	}

	handler := initializeTestEnv(t, ucp, dbClient)

	rgUCP.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, DB store.StorageClient, body []byte, path string) (rest.Response, error) {
		return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
	})

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"resourceProviders": map[string]string{
				"Applications.Core": "http://localhost:7443",
			},
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("PUT", baseURI+"/radius/local/resourceGroups/rg1", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	requireJSON(t, map[string]interface{}{}, w)
}

func Test_Handler_DeleteResourceGroup(t *testing.T) {
	ctrl := gomock.NewController(t)
	rgUCP := resourcegroups.NewMockResourceGroupsUCPHandler(ctrl)
	dbClient := store.NewMockStorageClient(ctrl)
	ucp := ucphandler.UCPHandler{
		Planes:         planes.NewMockPlanesUCPHandler(ctrl),
		ResourceGroups: rgUCP,
	}

	handler := initializeTestEnv(t, ucp, dbClient)

	rgUCP.EXPECT().DeleteByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, DB store.StorageClient, path string) (rest.Response, error) {
		return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
	})

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"resourceProviders": map[string]string{
				"Applications.Core": "http://localhost:7443",
			},
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("DELETE", baseURI+"/radius/local/resourceGroups/rg1", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	requireJSON(t, map[string]interface{}{}, w)
}

func Test_Handler_GetResourceGroup(t *testing.T) {
	ctrl := gomock.NewController(t)
	rgUCP := resourcegroups.NewMockResourceGroupsUCPHandler(ctrl)
	dbClient := store.NewMockStorageClient(ctrl)
	ucp := ucphandler.UCPHandler{
		Planes:         planes.NewMockPlanesUCPHandler(ctrl),
		ResourceGroups: rgUCP,
	}

	handler := initializeTestEnv(t, ucp, dbClient)

	rgUCP.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, DB store.StorageClient, path string) (rest.Response, error) {
		return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
	})

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"resourceProviders": map[string]string{
				"Applications.Core": "http://localhost:7443",
			},
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", baseURI+"/radius/local/resourceGroups/rg1", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	requireJSON(t, map[string]interface{}{}, w)
}

func Test_Handler_ListResourceGroups(t *testing.T) {
	ctrl := gomock.NewController(t)
	rgUCP := resourcegroups.NewMockResourceGroupsUCPHandler(ctrl)
	dbClient := store.NewMockStorageClient(ctrl)
	ucp := ucphandler.UCPHandler{
		Planes:         planes.NewMockPlanesUCPHandler(ctrl),
		ResourceGroups: rgUCP,
	}

	handler := initializeTestEnv(t, ucp, dbClient)

	rgUCP.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, DB store.StorageClient, path string) (rest.Response, error) {
		return rest.NewOKResponse(map[string]interface{}{}), nil // Empty JSON
	})

	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"resourceProviders": map[string]string{
				"Applications.Core": "http://localhost:7443",
			},
		},
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", baseURI+"/radius/local/resourceGroups", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	requireJSON(t, map[string]interface{}{}, w)
}
