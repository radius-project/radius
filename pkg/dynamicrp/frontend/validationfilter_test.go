/*
Copyright 2026 The Radius Authors.

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
	"strings"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestMakeValidationFilter(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		schema     map[string]any
		properties map[string]any
		invalid    bool
	}{
		{
			name: "valid enum",
			schema: map[string]any{"type": "object", "properties": map[string]any{
				"tls": map[string]any{"type": "string", "enum": []any{"required", "optional"}},
			}},
			properties: map[string]any{"tls": "required"},
		},
		{
			name: "invalid enum",
			schema: map[string]any{"type": "object", "properties": map[string]any{
				"tls": map[string]any{"type": "string", "enum": []any{"required", "optional"}},
			}},
			properties: map[string]any{"tls": "invalid"},
			invalid:    true,
		},
		{
			name: "invalid sensitive value",
			schema: map[string]any{"type": "object", "properties": map[string]any{
				"password": map[string]any{"type": "string", "maxLength": 2, "x-radius-sensitive": true},
			}},
			properties: map[string]any{"password": "private-value"},
			invalid:    true,
		},
		{name: "no schema", properties: map[string]any{"tls": "arbitrary"}},
		{name: "no schema or properties"},
		{name: "null properties", schema: map[string]any{"type": "object"}, invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ucp, err := createFakeUCPClientFactory(tt.schema)
			require.NoError(t, err)
			logCore, logs := observer.New(zap.DebugLevel)
			ctx := logr.NewContext(createTestContext(t), zapr.NewLogger(zap.New(logCore)))
			resource := &datamodel.DynamicResource{Properties: tt.properties}
			before, err := json.Marshal(resource)
			require.NoError(t, err)

			response, err := makeValidationFilter(ucp)(ctx, resource, nil, nil)
			require.NoError(t, err)
			if tt.invalid {
				badRequest, ok := response.(*rest.BadRequestResponse)
				require.True(t, ok)
				require.Equal(t, v1.CodeInvalidRequestContent, badRequest.Body.Error.Code)
				require.Contains(t, badRequest.Body.Error.Message, "Schema validation failed")
				body, err := json.Marshal(badRequest.Body)
				require.NoError(t, err)
				require.NotContains(t, string(body), "private-value")
			} else {
				require.Nil(t, response)
			}
			logJSON, err := json.Marshal(logs.All())
			require.NoError(t, err)
			require.NotContains(t, string(logJSON), "private-value")
			after, err := json.Marshal(resource)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
		})
	}
}

func TestMakeValidationFilter_SchemaFetchError(t *testing.T) {
	t.Parallel()
	ucp, err := testUCPClientFactoryWithError()
	require.NoError(t, err)
	response, err := makeValidationFilter(ucp)(createTestContext(t), &datamodel.DynamicResource{}, nil, nil)
	require.NoError(t, err)
	internal, ok := response.(*rest.InternalServerErrorResponse)
	require.True(t, ok)
	require.Equal(t, v1.CodeInternal, internal.Body.Error.Code)
	require.Equal(t, "Failed to fetch schema for request validation", internal.Body.Error.Message)
}

func TestMakeValidationFilter_NilClient(t *testing.T) {
	t.Parallel()
	_, err := makeValidationFilter(nil)(createTestContext(t), &datamodel.DynamicResource{}, nil, nil)
	require.ErrorContains(t, err, "UCP client is not configured")
}

func TestMakeValidationFilter_RequestAPIVersion(t *testing.T) {
	t.Parallel()
	calls := 0
	server := fake.APIVersionsServer{
		Get: func(ctx context.Context, plane, provider, resourceType, version string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
			calls++
			require.Equal(t, "local", plane)
			require.Equal(t, "Applications.Test", provider)
			require.Equal(t, "testResources", resourceType)
			require.Equal(t, testAPIVersion, version)
			resp.SetResponse(http.StatusOK, v20231001preview.APIVersionsClientGetResponse{
				Properties: &v20231001preview.APIVersionProperties{Schema: map[string]any{"type": "object"}},
			}, nil)
			return
		},
	}
	ucp, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		Transport: fake.NewAPIVersionsServerTransport(&server),
	})
	require.NoError(t, err)
	resource := &datamodel.DynamicResource{Properties: map[string]any{}}
	resource.UpdatedAPIVersion = "old-version"
	response, err := makeValidationFilter(ucp)(createTestContext(t), resource, nil, nil)
	require.NoError(t, err)
	require.Nil(t, response)
	require.Equal(t, 1, calls)
}

func TestDefaultAsyncPut_ValidationPreventsPersistence(t *testing.T) {
	t.Parallel()
	for _, existing := range []bool{false, true} {
		name := "create"
		if existing {
			name = "update"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mocks := gomock.NewController(t)
			storage := database.NewMockClient(mocks)
			status := statusmanager.NewMockStatusManager(mocks)
			if existing {
				storage.EXPECT().Get(gomock.Any(), testResourceID).Return(&database.Object{
					ID:   testResourceID,
					ETag: "original-etag",
					Data: map[string]any{
						"id": testResourceID, "provisioningState": "Succeeded",
						"properties": map[string]any{"tls": "required"},
					},
				}, nil)
			} else {
				storage.EXPECT().Get(gomock.Any(), testResourceID).Return(nil, &database.ErrNotFound{ID: testResourceID})
			}
			storage.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			status.EXPECT().QueueAsyncOperation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			ucp, err := createFakeUCPClientFactory(map[string]any{
				"type": "object", "properties": map[string]any{
					"tls": map[string]any{"type": "string", "enum": []any{"required", "optional"}, "default": "required"},
				},
			})
			require.NoError(t, err)
			controller, err := defaultoperation.NewDefaultAsyncPut(ctrl.Options{
				DatabaseClient: storage, StatusManager: status,
			}, ctrl.ResourceOptions[datamodel.DynamicResource]{
				RequestConverter:  converter.DynamicResourceDataModelFromVersioned,
				ResponseConverter: converter.DynamicResourceDataModelToVersioned,
				UpdateFilters: makeUpdateFilters(
					makeDefaultsFilter(ucp),
					makeValidationFilter(ucp),
					func(context.Context, *datamodel.DynamicResource, *datamodel.DynamicResource, *ctrl.Options) (rest.Response, error) {
						t.Fatal("encryption must not run for an invalid request")
						return nil, nil
					},
				),
			})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPut, testResourceID, strings.NewReader(`{"properties":{"tls":"invalid"}}`))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			ctx := createTestContext(t)
			response, err := controller.Run(ctx, recorder, req)
			require.NoError(t, err)
			require.NoError(t, response.Apply(ctx, recorder, req))
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), v1.CodeInvalidRequestContent)
			require.Empty(t, recorder.Header().Get("Azure-AsyncOperation"))
		})
	}
}
