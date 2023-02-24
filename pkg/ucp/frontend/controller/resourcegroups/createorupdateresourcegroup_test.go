// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
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

	input := v20220901privatepreview.ResourceGroupResource{
		Location: to.Ptr(v1.LocationGlobal),
	}

	body, err := json.Marshal(&input)
	require.NoError(t, err)

	url := "/planes/radius/local/resourceGroups/test-rg?api-version=2022-09-01-privatepreview"
	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	require.NoError(t, err)

	testResourceGroupID := "/planes/radius/local/resourceGroups/test-rg"
	testResourceGroupName := "test-rg"

	resourceGroup := datamodel.ResourceGroup{
		TrackedResource: v1.TrackedResource{
			ID:       testResourceGroupID,
			Name:     testResourceGroupName,
			Type:     ResourceGroupType,
			Location: v1.LocationGlobal,
			Tags:     map[string]string{},
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

	expectedResourceGroup := &v20220901privatepreview.ResourceGroupResource{
		ID:       &testResourceGroupID,
		Name:     &testResourceGroupName,
		Type:     to.Ptr(ResourceGroupType),
		Location: to.Ptr(v1.LocationGlobal),
		Tags:     *to.Ptr(map[string]*string{}),
	}
	expectedResponse := armrpc_rest.NewOKResponse(expectedResourceGroup)
	response, err := rgCtrl.Run(ctx, nil, request)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedResponse, response)
}

func Test_CreateResourceGroup_BadAPIVersion(t *testing.T) {
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
	url := "/planes/radius/local/resourceGroups/test-rg?api-version=notsupported"

	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	require.NoError(t, err)
	response, err := rgCtrl.Run(ctx, nil, request)
	require.NoError(t, err)
	expectedResponse := &armrpc_rest.BadRequestResponse{
		Body: v1.ErrorResponse{
			Error: v1.ErrorDetails{
				Code:    "BadRequest",
				Message: v1.ErrUnsupportedAPIVersion.Error(),
			},
		},
	}
	assert.DeepEqual(t, expectedResponse, response)
}
