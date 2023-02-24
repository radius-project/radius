// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	http "net/http"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_ListPlanes(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewListPlanes(ctrl.Options{
		StorageClient: mockStorageClient,
	})
	require.NoError(t, err)

	rootScope := "/planes"
	url := rootScope + "?api-version=2022-09-01-privatepreview"
	var query store.Query
	query.RootScope = rootScope
	query.IsScopeQuery = true

	expectedPlaneList := []any{}
	expectedResponse := armrpc_rest.NewOKResponse(
		&v1.PaginatedList{
			Value: expectedPlaneList,
		})

	mockStorageClient.EXPECT().Query(gomock.Any(), query).Return(&store.ObjectQueryResult{}, nil)

	request, err := testutil.GetARMTestHTTPRequestFromURL(context.Background(), http.MethodGet, url, nil)
	ctx := testutil.ARMTestContextFromRequest(request)
	require.NoError(t, err)
	actualResponse, err := planesCtrl.Run(ctx, nil, request)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}
