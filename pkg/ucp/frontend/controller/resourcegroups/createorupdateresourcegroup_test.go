// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"bytes"
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

func Test_CreateResourceGroup(t *testing.T) {
	ctx, cancel := testcontext.New(t)
	defer cancel()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	rgCtrl, err := NewCreateOrUpdateResourceGroup(ctrl.Options{
		DB: mockStorageClient,
	})
	require.NoError(t, err)

	body := []byte(`{
		"name": "test-rg"
	}`)
	path := "/planes/radius/local/resourceGroups/test-rg"

	testResourceGroupID := "/planes/radius/local/resourceGroups/test-rg"
	testResourceGroupName := "test-rg"

	resourceGroup := rest.ResourceGroup{
		ID:   testResourceGroupID,
		Name: testResourceGroupName,
	}

	o := &store.Object{
		Metadata: store.Metadata{
			ID: resourceGroup.ID,
		},
		Data: resourceGroup,
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})
	mockStorageClient.EXPECT().Save(gomock.Any(), o, gomock.Any())

	expectedResourceGroup := rest.ResourceGroup{
		ID:   testResourceGroupID,
		Name: testResourceGroupName,
	}
	expectedResponse := armrpc_rest.NewOKResponse(expectedResourceGroup)

	request, err := http.NewRequest(http.MethodPut, path, bytes.NewBuffer(body))
	require.NoError(t, err)
	response, err := rgCtrl.Run(ctx, nil, request)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, response)
}
