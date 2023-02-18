// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	http "net/http"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_ListPlanesByType(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewListPlanesByType(ctrl.Options{
		StorageClient: mockStorageClient,
	})
	require.NoError(t, err)

	rootScope := "/planes/radius"
	url := rootScope + "?api-version=2022-09-01-privatepreview"
	var query store.Query
	query.RootScope = "/planes"
	query.IsScopeQuery = true
	query.ResourceType = "radius"

	expectedPlaneList := []any{}
	expectedResponse := armrpc_rest.NewOKResponse(
		&v1.PaginatedList{
			Value: expectedPlaneList,
		})

	mockStorageClient.EXPECT().Query(gomock.Any(), query).Return(&store.ObjectQueryResult{}, nil)

	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	actualResponse, err := planesCtrl.Run(ctx, nil, request)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}
