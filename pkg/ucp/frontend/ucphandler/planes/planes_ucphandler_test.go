// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"gotest.tools/assert"
)

func Test_CreatePlane(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	var testHandler = NewPlanesUCPHandler(Options{})

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
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	plane := rest.Plane{
		ID:   "/planes/radius/local",
		Type: "System.Planes/radius",
		Name: "local",
		Properties: rest.PlaneProperties{
			ResourceProviders: map[string]string{
				"Applications.Core":       "http://localhost:9080/",
				"Applications.Connection": "http://localhost:9081/",
			},
			Kind: "UCPNative",
		},
	}

	var o store.Object
	o.Metadata.ContentType = "application/json"
	id := resources.UCPPrefix + plane.ID
	o.Metadata.ID = id
	o.Data, _ = json.Marshal(plane)

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any())
	mockStorageClient.EXPECT().Save(gomock.Any(), &o)
	_, err := testHandler.CreateOrUpdate(ctx, mockStorageClient, body, path)
	assert.Equal(t, nil, err)

}

func Test_ListPlane(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	path := "/planes"
	var testHandler = NewPlanesUCPHandler(Options{})

	var query store.Query
	query.RootScope = resources.UCPPrefix + path
	query.ScopeRecursive = true
	query.IsScopeQuery = true

	expectedPlaneList := rest.PlaneList{
		Value: []rest.Plane{},
	}
	expectedResponse := rest.NewOKResponse(expectedPlaneList)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	mockStorageClient.EXPECT().Query(gomock.Any(), query)
	actualResponse, err := testHandler.List(ctx, mockStorageClient, path)
	assert.Equal(t, nil, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}

func Test_GetPlaneByID(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	path := "/planes/radius/local"
	var testHandler = NewPlanesUCPHandler(Options{})

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	mockStorageClient.EXPECT().Get(ctx, gomock.Any())
	_, err := testHandler.GetByID(ctx, mockStorageClient, path)

	assert.Equal(t, nil, err)

}

func Test_DeletePlaneByID(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	path := "/planes/radius/local"
	var testHandler = NewPlanesUCPHandler(Options{})

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	mockStorageClient.EXPECT().Get(ctx, gomock.Any())
	mockStorageClient.EXPECT().Delete(ctx, gomock.Any())
	_, err := testHandler.DeleteByID(ctx, mockStorageClient, path)

	assert.Equal(t, nil, err)

}
