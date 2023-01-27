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
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_DeletePlaneByID(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewDeletePlane(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	url := "/planes/radius/local?api-version=2022-09-01-privatepreview"

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any())
	mockStorageClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any())

	request, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)

	expectedResponse := armrpc_rest.NewOKResponse(nil)

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, response)

}

func Test_DeletePlane_PlaneDoesNotExist(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewDeletePlane(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	url := "/planes/abc/xyz?api-version=2022-09-01-privatepreview"

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})

	request, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)

	expectedResponse := armrpc_rest.NewNoContentResponse()

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, response)
}
