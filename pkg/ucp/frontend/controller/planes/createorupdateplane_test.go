// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"

	armrpc_v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
)

func Test_CreateUCPNativePlane(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewCreateOrUpdatePlane(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	body := []byte(`{
		"properties": {
			"resourceProviders": {
				"Applications.Core": "http://localhost:9080/",
				"Applications.Connection": "http://localhost:9081/"
			},
			"kind": "UCPNative"
		}
	}`)
	url := "/planes/radius/local?api-version=2022-09-01-privatepreview"

	dataModelPlane := datamodel.Plane{
		TrackedResource: v1.TrackedResource{
			ID:   "/planes/radius/local",
			Type: "System.Planes/radius",
			Name: "local",
		},
		Properties: datamodel.PlaneProperties{
			ResourceProviders: map[string]*string{
				"Applications.Core":       to.Ptr("http://localhost:9080/"),
				"Applications.Connection": to.Ptr("http://localhost:9081/"),
			},
			Kind: rest.PlaneKindUCPNative,
		},
	}

	o := &store.Object{
		Metadata: store.Metadata{
			ID: dataModelPlane.TrackedResource.ID,
		},
		Data: dataModelPlane,
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any())
	mockStorageClient.EXPECT().Save(gomock.Any(), o, gomock.Any())

	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
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
		Type: to.Ptr("System.Planes/radius"),
	}

	expectedResponse := armrpc_rest.NewOKResponse(&versionedPlane)
	response, err := planesCtrl.Run(ctx, nil, request)
	require.NoError(t, err)

	assert.DeepEqual(t, expectedResponse, response)
}

func Test_CreateUCPNativePlane_NoResourceProviders(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewCreateOrUpdatePlane(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	body := []byte(`{
		"properties": {
			"kind": "UCPNative"
		}
	}`)
	url := "/planes/radius/local?api-version=2022-09-01-privatepreview"

	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)
	badResponse := &armrpc_rest.BadRequestResponse{
		Body: armrpc_v1.ErrorResponse{
			Error: armrpc_v1.ErrorDetails{
				Code:    armrpc_v1.CodeInvalid,
				Message: "$.properties.resourceProviders must be at least one provided.",
			},
		},
	}
	assert.DeepEqual(t, badResponse, response)
	require.NoError(t, err)
}

func Test_CreateAzurePlane_NoURL(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewCreateOrUpdatePlane(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	body := []byte(`{
		"properties": {
			"kind": "Azure"
		}
	}`)
	url := "/planes/azure/azurecloud?api-version=2022-09-01-privatepreview"

	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)
	badResponse := &armrpc_rest.BadRequestResponse{
		Body: armrpc_v1.ErrorResponse{
			Error: armrpc_v1.ErrorDetails{
				Code:    armrpc_v1.CodeInvalid,
				Message: "$.properties.URL must be non-empty string.",
			},
		},
	}
	assert.DeepEqual(t, badResponse, response)
	require.NoError(t, err)
}
