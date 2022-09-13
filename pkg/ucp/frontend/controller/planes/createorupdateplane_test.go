// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
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
	path := "/planes/radius/local"

	plane := rest.Plane{
		ID:   "/planes/radius/local",
		Type: "System.Planes/radius",
		Name: "local",
		Properties: rest.PlaneProperties{
			ResourceProviders: map[string]string{
				"Applications.Core":       "http://localhost:9080/",
				"Applications.Connection": "http://localhost:9081/",
			},
			Kind: rest.PlaneKindUCPNative,
		},
	}

	o := &store.Object{
		Metadata: store.Metadata{
			ID: plane.ID,
		},
		Data: plane,
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any())
	mockStorageClient.EXPECT().Save(gomock.Any(), o, gomock.Any())

	request, err := http.NewRequest(http.MethodPut, path, bytes.NewBuffer(body))
	require.NoError(t, err)

	expectedResponse := rest.NewOKResponse(plane)
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
	path := "/planes/radius/local"

	request, err := http.NewRequest(http.MethodPut, path, bytes.NewBuffer(body))
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)
	badResponse := &rest.BadRequestResponse{
		Body: rest.ErrorResponse{
			Error: rest.ErrorDetails{
				Code:    rest.Invalid,
				Message: "At least one resource provider must be configured for UCP native plane: local",
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
	path := "/planes/azure/azurecloud"

	request, err := http.NewRequest(http.MethodPut, path, bytes.NewBuffer(body))
	require.NoError(t, err)
	response, err := planesCtrl.Run(ctx, nil, request)
	badResponse := &rest.BadRequestResponse{
		Body: rest.ErrorResponse{
			Error: rest.ErrorDetails{
				Code:    rest.Invalid,
				Message: "URL must be specified for plane: azurecloud",
			},
		},
	}
	assert.DeepEqual(t, badResponse, response)
	require.NoError(t, err)
}

func Test_CreateAWSPlane(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewCreateOrUpdatePlane(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	body := []byte(`{}`)
	path := "/planes/aws/aws"

	plane := rest.AWSPlane{
		ID:         "/planes/aws/aws",
		Type:       "System.Planes/aws",
		Name:       "aws",
		Properties: rest.AWSPlaneProperties{},
	}

	o := &store.Object{
		Metadata: store.Metadata{
			ID:          plane.ID,
			ContentType: "application/json",
		},
		Data: &plane,
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})
	mockStorageClient.EXPECT().Save(gomock.Any(), o, gomock.Any())

	request, err := http.NewRequest(http.MethodPut, path, bytes.NewBuffer(body))
	require.NoError(t, err)

	expectedResponse := rest.NewOKResponse(plane)
	response, err := planesCtrl.Run(ctx, nil, request)

	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, response)
}
