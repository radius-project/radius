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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
)

func Test_GetPlaneByID(t *testing.T) {
	tCtx := testutil.NewTestContext(t)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewGetPlane(ctrl.Options{
		StorageClient: mockStorageClient,
	})
	require.NoError(t, err)

	url := "/planes/radius/local?api-version=2022-09-01-privatepreview"
	resourceID, err := resources.Parse(url)
	require.NoError(t, err)

	dbPlane := datamodel.Plane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID.String(),
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

	armctx := &v1.ARMRequestContext{
		ResourceID: resourceID,
		APIVersion: "2022-09-01-privatepreview",
	}
	ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

	response, err := planesCtrl.Run(ctx, nil, request)
	planeKind := v20220901privatepreview.PlaneKindUCPNative
	expectedResponse := armrpc_rest.NewOKResponse(&v20220901privatepreview.PlaneResource{
		ID:   to.Ptr(resourceID.String()),
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
	tCtx := testutil.NewTestContext(t)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewGetPlane(ctrl.Options{
		StorageClient: mockStorageClient,
	})
	require.NoError(t, err)

	url := "/planes/radius/local?api-version=2022-09-01-privatepreview"
	resourceID, err := resources.ParseScope(url)
	require.NoError(t, err)

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})

	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	armctx := &v1.ARMRequestContext{
		ResourceID: resourceID,
		APIVersion: "2022-09-01-privatepreview",
	}
	ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)
	response, err := planesCtrl.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedResponse := armrpc_rest.NewNotFoundResponse(resourceID)
	assert.DeepEqual(t, expectedResponse, response)
}
