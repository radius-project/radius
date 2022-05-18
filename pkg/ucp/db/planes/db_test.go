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

func TestSaveValidPlane(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
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

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	mockStorageClient.EXPECT().Save(ctx, &o).Return(nil)
	_, err := Save(ctx, mockStorageClient, plane)
	assert.Equal(t, nil, err)

}

func TestGetByIdPlane(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	id := "ucp:/planes/radius/local"
	resourceId, _ := resources.Parse(id)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	mockStorageClient.EXPECT().Get(ctx, resourceId)
	_, err := GetByID(ctx, mockStorageClient, resourceId)
	assert.Equal(t, nil, err)

}
