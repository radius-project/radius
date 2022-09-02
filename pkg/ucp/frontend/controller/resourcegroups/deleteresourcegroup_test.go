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
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_DeleteResourceGroupByID(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	path := "/planes/radius/local/resourceGroups/default"

	rgCtrl, err := NewDeleteResourceGroup(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	rg := rest.ResourceGroup{
		ID:   "/planes/radius/local/resourceGroups/default",
		Name: "default",
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &rg,
		}, nil
	})

	mockStorageClient.EXPECT().Query(gomock.Any(), gomock.Any())
	mockStorageClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any())

	request, err := http.NewRequest(http.MethodDelete, path, nil)
	require.NoError(t, err)
	// Issue Delete request
	response, err := rgCtrl.Run(ctx, nil, request)
	expectedResponse := rest.NewNoContentResponse()

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, response)

}

func Test_NonEmptyResourceGroup_CannotBeDeleted(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	path := "/planes/radius/local/resourceGroups/default"
	rgCtrl, err := NewDeleteResourceGroup(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	rg := rest.ResourceGroup{
		ID:   "/planes/radius/local/resourceGroups/default",
		Name: "default",
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &rg,
		}, nil
	})

	// This is corresponding to Get for all resources within the resource group
	envResource := rest.Resource{
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

	request, err := http.NewRequest(http.MethodDelete, path, nil)
	require.NoError(t, err)
	// Issue Delete request
	response, err := rgCtrl.Run(ctx, nil, request)
	conflictResponse := rest.NewConflictResponse("Resource group is not empty and cannot be deleted")

	assert.DeepEqual(t, conflictResponse, response)
	require.NoError(t, err)

}
