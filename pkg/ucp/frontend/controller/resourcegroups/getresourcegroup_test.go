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
	path := testResourceGroupID + "?api-version=2022-09-01-privatepreview"
	rg := datamodel.ResourceGroup{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       testResourceGroupID,
				Name:     testResourceGroupName,
				Type:     ResourceGroupType,
				Location: v1.LocationGlobal,
				Tags:     map[string]string{},
			},
		},
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
	expectedResourceGroup := v20220901privatepreview.ResourceGroupResource{
		ID:       &testResourceGroupID,
		Name:     &testResourceGroupName,
		Type:     to.Ptr(ResourceGroupType),
		Location: to.Ptr(v1.LocationGlobal),
		Tags:     *to.Ptr(map[string]*string{}),
	}
	expectedResponse := armrpc_rest.NewOKResponse(&expectedResourceGroup)
	assert.DeepEqual(t, expectedResponse, actualResponse)
}
