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
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_ListResourceGroups(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	rgCtrl, err := NewListResourceGroups(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	path := "/planes/radius/local/resourceGroups"

	query := store.Query{
		RootScope:    "/planes/radius/local",
		IsScopeQuery: true,
		ResourceType: "resourcegroups",
	}

	testResourceGroupID := "/planes/radius/local/resourceGroups/test-rg"
	testResourceGroupName := "test-rg"

	expectedResourceGroupList := rest.ResourceGroupList{
		Value: []rest.ResourceGroup{
			{
				ID:   testResourceGroupID,
				Name: testResourceGroupName,
			},
		},
	}
	expectedResponse := armrpc_rest.NewOKResponse(expectedResourceGroupList)

	rg := rest.ResourceGroup{
		ID:   testResourceGroupID,
		Name: testResourceGroupName,
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
	request, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	actualResponse, err := rgCtrl.Run(ctx, nil, request)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}
