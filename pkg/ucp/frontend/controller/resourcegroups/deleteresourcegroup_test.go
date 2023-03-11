// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func Test_DeleteResourceGroupByID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	w := httptest.NewRecorder()

	rgCtrl, err := NewDeleteResourceGroup(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	rgVersionedInput := &v20220901privatepreview.ResourceGroupResource{}
	resourceGroupInput := testutil.ReadFixture(createRequestBody)
	err = json.Unmarshal(resourceGroupInput, rgVersionedInput)
	require.NoError(t, err)

	request, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderFile, rgVersionedInput)
	require.NoError(t, err)

	rg := datamodel.ResourceGroup{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/planes/radius/local/resourceGroups/default",
				Name: "default",
				Type: ResourceGroupType,
			},
		},
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &rg,
		}, nil
	})

	mockStorageClient.EXPECT().Query(gomock.Any(), gomock.Any())
	mockStorageClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any())

	ctx := testutil.ARMTestContextFromRequest(request)
	// Issue Delete request
	response, err := rgCtrl.Run(ctx, w, request)
	require.NoError(t, err)
	err = response.Apply(ctx, w, request)
	require.NoError(t, err)

	result := w.Result()
	require.Equal(t, http.StatusOK, result.StatusCode)

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Empty(t, payload, "response body should be empty")

}

func Test_DeleteNonExistentResourceGroup(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	w := httptest.NewRecorder()

	rgCtrl, err := NewDeleteResourceGroup(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	rgVersionedInput := &v20220901privatepreview.ResourceGroupResource{}
	resourceGroupInput := testutil.ReadFixture(createRequestBody)
	err = json.Unmarshal(resourceGroupInput, rgVersionedInput)
	require.NoError(t, err)

	request, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderFileNonExistent, rgVersionedInput)
	require.NoError(t, err)

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})

	ctx := testutil.ARMTestContextFromRequest(request)
	// Issue Delete request
	response, err := rgCtrl.Run(ctx, w, request)
	require.NoError(t, err)
	err = response.Apply(ctx, w, request)
	require.NoError(t, err)

	result := w.Result()
	require.Equal(t, http.StatusNoContent, result.StatusCode)

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Empty(t, payload, "response body should be empty")

}

func Test_NonEmptyResourceGroup_CannotBeDeleted(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	w := httptest.NewRecorder()

	// This is corresponding to Get for all resources within the resource group
	envResource := datamodel.Resource{
		ID:   "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/my-env",
		Name: "my-env",
		Type: "Applications.Core/environments",
	}

	rg := datamodel.ResourceGroup{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/planes/radius/local/resourceGroups/test-rg",
				Name: "test-rg",
				Type: "",
			},
		},
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &rg,
		}, nil
	})

	mockStorageClient.EXPECT().Query(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
		return &store.ObjectQueryResult{
			Items: []store.Object{
				{
					Metadata: store.Metadata{},
					Data:     &envResource,
				},
			},
		}, nil
	})

	rgCtrl, err := NewDeleteResourceGroup(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	rgVersionedInput := &v20220901privatepreview.ResourceGroupResource{}
	resourceGroupInput := testutil.ReadFixture(createRequestBody)
	err = json.Unmarshal(resourceGroupInput, rgVersionedInput)
	require.NoError(t, err)

	request, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderFile, rgVersionedInput)
	require.NoError(t, err)

	ctx := testutil.ARMTestContextFromRequest(request)
	// Issue Delete request
	response, err := rgCtrl.Run(ctx, w, request)
	conflictResponse := armrpc_rest.NewConflictResponse("Resource group is not empty and cannot be deleted")

	require.Equal(t, conflictResponse, response)
	require.NoError(t, err)

}
