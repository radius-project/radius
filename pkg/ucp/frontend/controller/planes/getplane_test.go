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
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
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

	url := "/planes/radius/local?api-version=2022-09-01-privatepreview"

	dbPlane := datamodel.Plane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/planes/radius/local",
				Type: "radius",
				Name: "local",
			},
		},
		Properties: datamodel.PlaneProperties{
			Kind: rest.PlaneKindUCPNative,
			ResourceProviders: map[string]*string{
				"Applications.Core": to.Ptr("http://localhost:8080"),
			},
		},
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &dbPlane,
		}, nil
	})

	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)
	planeKind := v20220901privatepreview.PlaneKindUCPNative
	expectedResponse := armrpc_rest.NewOKResponse(&v20220901privatepreview.PlaneResource{
		ID:   to.Ptr("/planes/radius/local"),
		Type: to.Ptr("radius"),
		Name: to.Ptr("local"),
		Properties: &v20220901privatepreview.PlaneResourceProperties{
			Kind: &planeKind,
			ResourceProviders: map[string]*string{
				"Applications.Core": to.Ptr("http://localhost:8080"),
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

	url := "/planes/radius/local?api-version=2022-09-01-privatepreview"

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})

	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)
	require.NoError(t, err)

	id, err := resources.ParseScope("/planes/radius/local")
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewNotFoundResponse(id)
	assert.DeepEqual(t, expectedResponse, response)
}
