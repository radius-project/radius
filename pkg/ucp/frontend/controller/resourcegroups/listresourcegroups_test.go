// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	http "net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
)

func Test_ListResourceGroups(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	rgCtrl, err := NewListResourceGroups(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	url := "/planes/radius/local/resourceGroups?api-version=2023-04-15-preview"

	query := store.Query{
		RootScope:    "/planes/radius/local",
		IsScopeQuery: true,
		ResourceType: "resourcegroups",
	}

	testResourceGroupID := "/planes/radius/local/resourceGroups/test-rg"
	testResourceGroupName := "test-rg"

	rg := datamodel.ResourceGroup{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       testResourceGroupID,
				Name:     testResourceGroupName,
				Type:     ResourceGroupType,
				Location: v1.LocationGlobal,
			},
		},
	}

	mockStorageClient.EXPECT().Query(gomock.Any(), query).DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
		return &store.ObjectQueryResult{
			Items: []store.Object{
				{
					Metadata: store.Metadata{},
					Data:     &rg,
				},
			},
		}, nil
	})
	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)
	actualResponse, err := rgCtrl.Run(ctx, nil, request)
	require.NoError(t, err)

	resourceGroup := v20230415preview.ResourceGroupResource{
		ID:       &testResourceGroupID,
		Name:     &testResourceGroupName,
		Type:     to.Ptr(ResourceGroupType),
		Location: to.Ptr(v1.LocationGlobal),
		Tags:     *to.Ptr(map[string]*string{}),
	}
	expectedResourceGroupList := &v1.PaginatedList{
		Value: []any{
			&resourceGroup,
		},
	}
	expectedResponse := armrpc_rest.NewOKResponse(expectedResourceGroupList)

	require.Equal(t, expectedResponse, actualResponse)
}
