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
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_GetPlaneByID(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewGetPlane(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	path := "/planes/radius/local"

	dbPlane := rest.Plane{
		ID:   "/planes/radius/local",
		Type: "radius",
		Name: "local",
		Properties: rest.PlaneProperties{
			Kind: rest.PlaneKindUCPNative,
			ResourceProviders: map[string]string{
				"Applications.Core": "http://localhost:8080",
			},
		},
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &dbPlane,
		}, nil
	})

	request, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)
	expectedResponse := armrpc_rest.NewOKResponse(rest.Plane{
		ID:   "/planes/radius/local",
		Type: "radius",
		Name: "local",
		Properties: rest.PlaneProperties{
			Kind: rest.PlaneKindUCPNative,
			ResourceProviders: map[string]string{
				"Applications.Core": "http://localhost:8080",
			},
		},
	})

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, response)
}

func Test_GetPlaneByID_PlaneDoesNotExist(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewGetPlane(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	path := "/planes/radius/local"

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})

	request, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)
	require.NoError(t, err)

	id, err := resources.ParseScope("/planes/radius/local")
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewNotFoundResponse(id)
	assert.DeepEqual(t, expectedResponse, response)
}
