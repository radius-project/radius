// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_DeleteResourceGroupByID(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	url := "/planes/radius/local/resourceGroups/default?api-version=2022-09-01-privatepreview"

	rgCtrl, err := NewDeleteResourceGroup(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
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

	request, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	// Issue Delete request
	response, err := rgCtrl.Run(ctx, nil, request)
	expectedResponse := armrpc_rest.NewOKResponse(nil)

	require.NoError(t, err)
	require.Equal(t, expectedResponse, response)

}

func Test_NonEmptyResourceGroup_CannotBeDeleted(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	url := "/planes/radius/local/resourceGroups/default?api-version=2022-09-01-privatepreview"
	rgCtrl, err := NewDeleteResourceGroup(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
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

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &rg,
		}, nil
	})

	// This is corresponding to Get for all resources within the resource group
	envResource := datamodel.Resource{
		ID:   "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/my-env",
		Name: "my-env",
		Type: "Applications.Core/environments",
	}

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

	request, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	// Issue Delete request
	response, err := rgCtrl.Run(ctx, nil, request)
	conflictResponse := armrpc_rest.NewConflictResponse("Resource group is not empty and cannot be deleted")

	require.Equal(t, conflictResponse, response)
	require.NoError(t, err)

}
