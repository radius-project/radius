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
	"context"
	http "net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/database"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

func Test_ListResourceGroups(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockDatabaseClient := database.NewMockClient(mockCtrl)

	rgCtrl, err := NewListResourceGroups(armrpc_controller.Options{DatabaseClient: mockDatabaseClient})
	require.NoError(t, err)

	url := "/planes/radius/local/resourceGroups?api-version=2023-10-01-preview"

	query := database.Query{
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

	mockDatabaseClient.EXPECT().Query(gomock.Any(), query).DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
		return &database.ObjectQueryResult{
			Items: []database.Object{
				{
					Metadata: database.Metadata{},
					Data:     &rg,
				},
			},
		}, nil
	})
	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(request)
	actualResponse, err := rgCtrl.Run(ctx, nil, request)
	require.NoError(t, err)

	resourceGroup := v20231001preview.ResourceGroupResource{
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
