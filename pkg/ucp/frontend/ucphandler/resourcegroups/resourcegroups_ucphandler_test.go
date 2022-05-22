// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_CreateResourceGroup(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	var testHandler = NewResourceGroupsUCPHandler()

	body := []byte(`{
		"name": "test-rg"
	}`)
	path := "/planes/radius/local/resourceGroups/test-rg"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	resourceGroup := rest.ResourceGroup{
		ID:   "/planes/radius/local/resourceGroups/test-rg",
		Name: "test-rg",
	}

	var o store.Object
	o.Metadata.ContentType = "application/json"
	id := resources.UCPPrefix + resourceGroup.ID
	o.Metadata.ID = id
	o.Data, _ = json.Marshal(resourceGroup)

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any())
	mockStorageClient.EXPECT().Save(gomock.Any(), &o)
	_, err := testHandler.Create(ctx, mockStorageClient, body, path)
	assert.Equal(t, nil, err)

}

func Test_ListResourceGroups(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	path := "/planes/radius/local/resourceGroups"
	var testHandler = NewResourceGroupsUCPHandler()

	var query store.Query
	query.RootScope = resources.UCPPrefix + path
	query.ScopeRecursive = false
	query.IsScopeQuery = true

	testResourceGroupID := "/planes/radius/local/resourceGroups/test-rg"
	testResourceGroupName := "test-rg"

	expectedResourceGroupList := rest.ResourceGroupList{
		Value: []rest.ResourceGroup{
			{
				ID:   testResourceGroupID,
				Name: testResourceGroupName,
			},
		},
	}
	expectedResponse := rest.NewOKResponse(expectedResourceGroupList)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	rg := rest.ResourceGroup{
		ID:   testResourceGroupID,
		Name: testResourceGroupName,
	}
	bytes, err := json.Marshal(rg)
	require.NoError(t, err)
	mockStorageClient.EXPECT().Query(gomock.Any(), query).DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) ([]store.Object, error) {
		return []store.Object{
			{
				Metadata: store.Metadata{},
				Data:     bytes,
			},
		}, nil
	})
	actualResponse, err := testHandler.List(ctx, mockStorageClient, path)
	assert.Equal(t, nil, err)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}

func Test_GetResourceGroupByID(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	testResourceGroupID := "/planes/radius/local/resourceGroups/test-rg"
	testResourceGroupName := "test-rg"
	path := testResourceGroupID
	var testHandler = NewResourceGroupsUCPHandler()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	rg := rest.ResourceGroup{
		ID:   testResourceGroupID,
		Name: testResourceGroupName,
	}
	bytes, err := json.Marshal(rg)
	require.NoError(t, err)
	mockStorageClient.EXPECT().Get(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, id resources.ID, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     bytes,
		}, nil
	})

	actualResponse, err := testHandler.GetByID(ctx, mockStorageClient, path)

	assert.Equal(t, nil, err)
	expectedResourceGroup := rest.ResourceGroup{
		ID:   testResourceGroupID,
		Name: testResourceGroupName,
	}
	expectedResponse := rest.NewOKResponse(expectedResourceGroup)
	assert.DeepEqual(t, expectedResponse, actualResponse)

}

func Test_DeleteResourceGroupByID(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	path := "/planes/radius/local/resourceGroups/test-rg"
	var testHandler = NewResourceGroupsUCPHandler()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	mockStorageClient.EXPECT().Get(ctx, gomock.Any())
	mockStorageClient.EXPECT().Delete(ctx, gomock.Any())
	_, err := testHandler.DeleteByID(ctx, mockStorageClient, path)

	assert.Equal(t, nil, err)

}
