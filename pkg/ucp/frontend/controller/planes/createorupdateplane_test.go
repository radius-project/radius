// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"

	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func Test_CreateUCPNativePlane(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	w := httptest.NewRecorder()

	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewCreateOrUpdatePlane(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	dataModelPlane := datamodel.Plane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/planes/radius/local",
				Type:     "System.Planes/radius",
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

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})
	mockStorageClient.EXPECT().Save(gomock.Any(), o, gomock.Any())

	planeVersionedInput := &v20220901privatepreview.PlaneResource{}
	planeInput := testutil.ReadFixture(createRequestBody)
	err = json.Unmarshal(planeInput, planeVersionedInput)
	require.NoError(t, err)

	request, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPut, testHeaderFile, planeVersionedInput)
	require.NoError(t, err)

	versionedPlane := v20220901privatepreview.PlaneResource{
		Properties: &v20220901privatepreview.PlaneResourceProperties{
			Kind: to.Ptr(v20220901privatepreview.PlaneKindUCPNative),
			ResourceProviders: map[string]*string{
				"Applications.Connection": to.Ptr("http://localhost:9081/"),
				"Applications.Core":       to.Ptr("http://localhost:9080/"),
			},
		},
		ID:   to.Ptr("/planes/radius/local"),
		Name: to.Ptr("local"),
		Type: to.Ptr(""),
	}

	headers := map[string]string{"ETag": ""}

	_ = armrpc_rest.NewOKResponseWithHeaders(versionedPlane, headers)

	ctx := testutil.ARMTestContextFromRequest(request)
	response, err := planesCtrl.Run(ctx, w, request)
	require.NoError(t, err)
	_ = response.Apply(ctx, w, request)

	actualOutput := v20220901privatepreview.PlaneResource{}
	err = json.Unmarshal(w.Body.Bytes(), &actualOutput)
	require.NoError(t, err)
	require.Equal(t, versionedPlane, actualOutput)
}

func Test_CreateUCPNativePlane_NoResourceProviders(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewCreateOrUpdatePlane(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	planeVersionedInput := &v20220901privatepreview.PlaneResource{}
	planeInput := testutil.ReadFixture(createRequestWithNoProvidersBody)
	err = json.Unmarshal(planeInput, planeVersionedInput)
	require.NoError(t, err)

	request, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPut, testHeaderFile, planeVersionedInput)
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)
	response, err := planesCtrl.Run(ctx, nil, request)
	require.Nil(t, response)
	expectedError := &v1.ErrModelConversion{
		PropertyName: "$.properties.resourceProviders",
		ValidValue:   "at least one provided",
	}
	require.Equal(t, expectedError, err)
}

func Test_CreateAzurePlane_NoURL(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewCreateOrUpdatePlane(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	planeVersionedInput := &v20220901privatepreview.PlaneResource{}
	planeInput := testutil.ReadFixture(createRequestWithNoUrlBody)
	err = json.Unmarshal(planeInput, planeVersionedInput)
	require.NoError(t, err)

	request, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPut, testHeaderFileAzure, planeVersionedInput)
	require.NoError(t, err)
	ctx := testutil.ARMTestContextFromRequest(request)
	response, err := planesCtrl.Run(ctx, nil, request)
	require.Nil(t, response)
	expectedError := &v1.ErrModelConversion{
		PropertyName: "$.properties.URL",
		ValidValue:   "non-empty string",
	}

	require.Equal(t, expectedError, err)
}
