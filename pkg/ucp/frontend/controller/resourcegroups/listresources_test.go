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
package resourcegroups

import (
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
)

func Test_ListResources(t *testing.T) {
	entryResource := v20220901privatepreview.GenericResource{
		ID:   to.Ptr("/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/applications/test-app"),
		Type: to.Ptr("Applications.Core/applications"),
		Name: to.Ptr("test-app"),
	}
	entryDatamodel := datamodel.GenericResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "ignored",
				Type: "ignored",
				Name: "ignored",
			},
		},
		Properties: datamodel.GenericResourceProperties{
			ID:   *entryResource.ID,
			Type: *entryResource.Type,
			Name: *entryResource.Name,
		},
	}

	// Not currently used, but may be in the future.
	resourceGroupDatamodel := datamodel.ResourceGroup{}

	resourceGroupID := "/planes/radius/local/resourceGroups/test-rg"
	id := resourceGroupID + "/resources"

	t.Run("success", func(t *testing.T) {
		storage, ctrl := setupListResources(t)

		storage.EXPECT().
			Get(gomock.Any(), resourceGroupID).
			Return(&store.Object{Data: resourceGroupDatamodel}, nil).
			Times(1)

		expectedQuery := store.Query{RootScope: resourceGroupID, ResourceType: v20220901privatepreview.ResourceType}
		storage.EXPECT().
			Query(gomock.Any(), expectedQuery).
			Return(&store.ObjectQueryResult{Items: []store.Object{{Data: entryDatamodel}}}, nil).
			Times(1)

		expected := armrpc_rest.NewOKResponse(&v1.PaginatedList{
			Value: []any{&entryResource},
		})

		request, err := http.NewRequest(http.MethodGet, ctrl.Options().PathBase+id+"?api-version="+v20220901privatepreview.Version, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(request)
		response, err := ctrl.Run(ctx, nil, request)
		require.NoError(t, err)
		require.Equal(t, expected, response)
	})

	t.Run("success - empty", func(t *testing.T) {
		storage, ctrl := setupListResources(t)

		storage.EXPECT().
			Get(gomock.Any(), resourceGroupID).
			Return(&store.Object{Data: resourceGroupDatamodel}, nil).
			Times(1)

		expectedQuery := store.Query{RootScope: resourceGroupID, ResourceType: v20220901privatepreview.ResourceType}
		storage.EXPECT().
			Query(gomock.Any(), expectedQuery).
			Return(&store.ObjectQueryResult{Items: []store.Object{}}, nil).
			Times(1)

		expected := armrpc_rest.NewOKResponse(&v1.PaginatedList{})

		request, err := http.NewRequest(http.MethodGet, ctrl.Options().PathBase+id+"?api-version="+v20220901privatepreview.Version, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(request)
		response, err := ctrl.Run(ctx, nil, request)
		require.NoError(t, err)
		require.Equal(t, expected, response)
	})

	t.Run("resource group not found", func(t *testing.T) {
		storage, ctrl := setupListResources(t)

		storage.EXPECT().
			Get(gomock.Any(), resourceGroupID).
			Return(nil, &store.ErrNotFound{ID: resourceGroupID}).
			Times(1)

		parsed, err := resources.Parse(id)
		require.NoError(t, err)

		expected := armrpc_rest.NewNotFoundResponse(parsed)

		request, err := http.NewRequest(http.MethodGet, ctrl.Options().PathBase+id+"?api-version="+v20220901privatepreview.Version, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(request)
		response, err := ctrl.Run(ctx, nil, request)
		require.NoError(t, err)
		require.Equal(t, expected, response)
	})
}

func setupListResources(t *testing.T) (*store.MockStorageClient, *ListResources) {
	ctrl := gomock.NewController(t)
	storage := store.NewMockStorageClient(ctrl)

	c, err := NewListResources(armrpc_controller.Options{StorageClient: storage, PathBase: "/" + uuid.New().String()})
	require.NoError(t, err)

	return storage, c.(*ListResources)
}
