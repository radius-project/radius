// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_GetResourceGroupByID(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	rgCtrl, err := NewGetResourceGroup(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	testResourceGroupID := "/planes/radius/local/resourceGroups/test-rg"
	testResourceGroupName := "test-rg"
	path := testResourceGroupID
	rg := rest.ResourceGroup{
		ID:   testResourceGroupID,
		Name: testResourceGroupName,
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return &store.Object{
			Metadata: store.Metadata{},
			Data:     &rg,
		}, nil
	})

	request, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	actualResponse, err := rgCtrl.Run(ctx, nil, request)

	require.NoError(t, err)
	expectedResourceGroup := rest.ResourceGroup{
		ID:   testResourceGroupID,
		Name: testResourceGroupName,
	}
	expectedResponse := armrpc_rest.NewOKResponse(expectedResourceGroup)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}
