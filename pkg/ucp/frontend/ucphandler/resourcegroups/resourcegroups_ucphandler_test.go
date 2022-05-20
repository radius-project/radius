// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
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
	var testHandler = NewResourceGroupsUCPHandler(Options{})

	body := []byte(`{
		"name": "test-rg"
	}`)
	path := "/planes/radius/local/resourceGroups/test-rg"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})

	resourceGroup := rest.ResourceGroup{
		ID:                "/planes/radius/local/resourceGroups/test-rg",
		Name:              "test-rg",
		ProvisioningState: rest.ProvisioningStateSucceeded,
	}

	id := resources.UCPPrefix + resourceGroup.ID
	o := store.Object{
		Metadata: store.Metadata{
			ContentType: "application/json",
			ID:          id,
		},
		Data: &resourceGroup,
	}

	mockStorageClient.EXPECT().Save(gomock.Any(), &o)
	_, err := testHandler.Create(ctx, mockStorageClient, body, path)
	assert.Equal(t, nil, err)

}

func Test_CreateResourceGroup_Conflict(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	var testHandler = NewResourceGroupsUCPHandler(Options{})

	body := []byte(`{
	}`)
	path := "/planes/radius/local/resourceGroups/test-rg"

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	resourceGroup := rest.ResourceGroup{
		ID:                "/planes/radius/local/resourceGroups/test-rg",
		Name:              "test-rg",
		ProvisioningState: rest.ProvisioningStateDeleting,
	}
	mockStorageClient.EXPECT().Get(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &resourceGroup,
		}, nil
	})

	response, err := testHandler.Create(ctx, mockStorageClient, body, path)
	assert.Equal(t, nil, err)
	conflictResponse := rest.NewConflictResponse("Cannot create/update resource group while delete is in progress")
	assert.DeepEqual(t, conflictResponse, response)

}

func Test_ListResourceGroups(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	path := "/planes/radius/local/resourceGroups"
	var testHandler = NewResourceGroupsUCPHandler(Options{})

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

	mockStorageClient.EXPECT().Query(gomock.Any(), query).DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
		return &store.ObjectQueryResult{
			Items: []store.Object{
				{
					Metadata: store.Metadata{},
					Data:     &rg,
				},
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
	var testHandler = NewResourceGroupsUCPHandler(Options{})

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)
	rg := rest.ResourceGroup{
		ID:   testResourceGroupID,
		Name: testResourceGroupName,
	}

	mockStorageClient.EXPECT().Get(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &rg,
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
	path := "/planes/radius/local/resourceGroups/default"
	client := httpClientWithRoundTripper(http.StatusOK, "OK")

	var testHandler = NewResourceGroupsUCPHandler(Options{
		Client: client,
	})

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	rg := rest.ResourceGroup{
		ID:                "/planes/radius/local/resourceGroups/default",
		Name:              "default",
		ProvisioningState: rest.ProvisioningStateSucceeded,
	}

	id := resources.UCPPrefix + rg.ID
	o := store.Object{
		Metadata: store.Metadata{
			ContentType: "application/json",
			ID:          id,
		},
		Data: &rg,
	}

	mockStorageClient.EXPECT().Get(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &rg,
		}, nil
	})
	rg.ProvisioningState = rest.ProvisioningStateDeleting
	mockStorageClient.EXPECT().Save(ctx, &o)

	// This is corresponding to Get for all resources within the resource group
	envResource := rest.Resource{
		ID:   "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/my-env",
		Name: "my-env",
		Type: "Applications.Core/environments",
	}

	mockStorageClient.EXPECT().Query(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
		return &store.ObjectQueryResult{
			Items: []store.Object{
				{
					Metadata: store.Metadata{},
					Data:     &envResource,
				},
			},
		}, nil
	})

	// This is corresponding to Get plane to read providers for the plane
	plane := rest.Plane{
		ID:   "/planes/radius/local",
		Name: "local",
		Properties: rest.PlaneProperties{
			ResourceProviders: map[string]string{
				"Applications.Core": "http://localhost:9000",
			},
		},
	}
	mockStorageClient.EXPECT().Get(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &plane,
		}, nil
	})

	mockStorageClient.EXPECT().Delete(ctx, gomock.Any())
	request, err := http.NewRequest(http.MethodDelete, "/planes/radius/local", nil)
	require.NoError(t, err)

	// Run a mock server for the RP. A delete call will be made to the RP.
	httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Issue Delete request
	_, err = testHandler.DeleteByID(ctx, mockStorageClient, path, request)

	assert.Equal(t, nil, err)

}

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func httpClientWithRoundTripper(statusCode int, response string) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: statusCode,
				Body:       ioutil.NopCloser(bytes.NewBufferString(response)),
			}
		}),
	}
}
