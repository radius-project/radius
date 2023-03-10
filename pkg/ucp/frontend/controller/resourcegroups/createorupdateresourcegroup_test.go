// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
)

func Test_CreateResourceGroup(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	w := httptest.NewRecorder()

	rgCtrl, err := NewCreateOrUpdateResourceGroup(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	rgVersionedInput := &v20220901privatepreview.ResourceGroupResource{}
	resourceGroupInput := testutil.ReadFixture(createRequestBody)
	err = json.Unmarshal(resourceGroupInput, rgVersionedInput)
	require.NoError(t, err)

	request, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPut, testHeaderFile, rgVersionedInput)
	require.NoError(t, err)

	testResourceGroupID := "/planes/radius/local/resourceGroups/test-rg"
	testResourceGroupName := "test-rg"

	resourceGroup := datamodel.ResourceGroup{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       testResourceGroupID,
				Name:     testResourceGroupName,
				Type:     ResourceGroupType,
				Location: "West US",
				Tags:     map[string]string{},
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      "2022-09-01-privatepreview",
				UpdatedAPIVersion:      "2022-09-01-privatepreview",
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
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

	versionedResourceGroup := v20220901privatepreview.ResourceGroupResource{
		ID:       &testResourceGroupID,
		Name:     &testResourceGroupName,
		Type:     to.Ptr("System.Resources/resourceGroups"),
		Location: to.Ptr("West US"),
		Tags:     *to.Ptr(map[string]*string{}),
	}

	ctx := testutil.ARMTestContextFromRequest(request)
	response, err := rgCtrl.Run(ctx, w, request)
	require.NoError(t, err)
	_ = response.Apply(ctx, w, request)

	actualOutput := v20220901privatepreview.ResourceGroupResource{}
	err = json.Unmarshal(w.Body.Bytes(), &actualOutput)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, response)
}

func Test_CreateResourceGroup_BadAPIVersion(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	w := httptest.NewRecorder()

	rgCtrl, err := NewCreateOrUpdateResourceGroup(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	rgVersionedInput := &v20220901privatepreview.ResourceGroupResource{}
	resourceGroupInput := testutil.ReadFixture(createRequestBody)
	err = json.Unmarshal(resourceGroupInput, rgVersionedInput)
	require.NoError(t, err)

	request, err := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodPut, testHeaderFileWithBadAPIVersion, rgVersionedInput)
	require.NoError(t, err)

	ctx := testutil.ARMTestContextFromRequest(request)
	response, err := rgCtrl.Run(ctx, w, request)
	require.Nil(t, response)
	require.ErrorIs(t, v1.ErrUnsupportedAPIVersion, err)
}
