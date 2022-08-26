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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
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

	resourceGroup := datamodel.ResourceGroup{
		TrackedResource: v1.TrackedResource{
			ID:   testResourceGroupID,
			Name: testResourceGroupName,
		},
	}

	o := &store.Object{
		Metadata: store.Metadata{
			ID: resourceGroup.TrackedResource.ID,
		},
		Data: &resourceGroup,
	}

	mockStorageClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
		return nil, &store.ErrNotFound{}
	})
	mockStorageClient.EXPECT().Save(gomock.Any(), o, gomock.Any())

	expectedResourceGroup := v20220901privatepreview.ResourceGroupResource{
		ID:   &testResourceGroupID,
		Name: &testResourceGroupName,
		Type: to.Ptr(""),
	}
	expectedResponse := rest.NewOKResponse(&expectedResourceGroup)

	request, err := http.NewRequest(http.MethodPut, path, bytes.NewBuffer(body))
	require.NoError(t, err)
	response, err := rgCtrl.Run(ctx, nil, request)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, response)
}
