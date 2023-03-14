// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"io"
	http "net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func Test_DeletePlaneByID(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	w := httptest.NewRecorder()
	request, _ := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderFile, nil)
	ctx := testutil.ARMTestContextFromRequest(request)

	dataModelPlane := datamodel.Plane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/planes/radius/local",
				Type:     "",
				Name:     "local",
				Location: "West US",
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      "2022-09-01-privatepreview",
				UpdatedAPIVersion:      "2022-09-01-privatepreview",
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
		},
		Properties: datamodel.PlaneProperties{
			ResourceProviders: map[string]*string{
				"Applications.Connection": to.Ptr("http://localhost:9081/"),
				"Applications.Core":       to.Ptr("http://localhost:9080/"),
			},
			Kind: rest.PlaneKindUCPNative,
		},
	}

	o := &store.Object{
		Metadata: store.Metadata{
			ID: dataModelPlane.TrackedResource.ID,
		},
		Data: &dataModelPlane,
	}

	mockStorageClient.
		EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
			return o, nil
		})

	mockStorageClient.
		EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any())

	opts := ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	}
	ctl, err := NewDeletePlane(opts)

	require.NoError(t, err)
	resp, err := ctl.Run(ctx, w, request)
	require.NoError(t, err)
	err = resp.Apply(ctx, w, request)
	require.NoError(t, err)

	result := w.Result()
	require.Equal(t, http.StatusOK, result.StatusCode)

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Empty(t, payload, "response body should be empty")
}

func Test_DeletePlane_PlaneDoesNotExist(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	w := httptest.NewRecorder()
	request, _ := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderFileNonExistentPlane, nil)
	ctx := testutil.ARMTestContextFromRequest(request)

	mockStorageClient.
		EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
			return nil, &store.ErrNotFound{}
		})

	opts := ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	}

	ctl, err := NewDeletePlane(opts)

	require.NoError(t, err)
	resp, err := ctl.Run(ctx, w, request)
	require.NoError(t, err)
	err = resp.Apply(ctx, w, request)
	require.NoError(t, err)

	result := w.Result()
	require.Equal(t, http.StatusNoContent, result.StatusCode)

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Empty(t, payload, "response body should be empty")
}
