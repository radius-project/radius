/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package planes

import (
	http "net/http"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_ListPlanesByType(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	ctrl := &ListPlanesByType[*datamodel.RadiusPlane, datamodel.RadiusPlane]{
		Operation: armrpc_controller.NewOperation[*datamodel.RadiusPlane, datamodel.RadiusPlane](
			armrpc_controller.Options{StorageClient: mockStorageClient},
			armrpc_controller.ResourceOptions[datamodel.RadiusPlane]{
				ResponseConverter: converter.RadiusPlaneDataModelToVersioned,
			}),
	}

	url := "/planes/radius?api-version=2023-10-01-preview"

	query := store.Query{
		RootScope:    "/planes",
		IsScopeQuery: true,
		ResourceType: "radius",
	}

	testPlaneId := "/planes/radius/local"
	testPlaneName := "local"
	testPlaneType := datamodel.RadiusPlaneResourceType

	planeData := datamodel.RadiusPlane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       testPlaneId,
				Name:     testPlaneName,
				Type:     testPlaneType,
				Location: "global",
			},
		},
		Properties: datamodel.RadiusPlaneProperties{
			ResourceProviders: map[string]string{
				"Applications.Core": "https://applications-rp",
			},
		},
	}

	mockStorageClient.EXPECT().Query(gomock.Any(), query).Return(&store.ObjectQueryResult{
		Items: []store.Object{
			{
				Metadata: store.Metadata{},
				Data:     &planeData,
			},
		},
	}, nil)

	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(request)
	actualResponse, err := ctrl.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedPlane := v20231001preview.RadiusPlaneResource{
		ID:       &testPlaneId,
		Name:     &testPlaneName,
		Type:     &testPlaneType,
		Location: to.Ptr("global"),
		Tags:     map[string]*string{},
		Properties: &v20231001preview.RadiusPlaneResourceProperties{
			ProvisioningState: to.Ptr(v20231001preview.ProvisioningState("Succeeded")),
			ResourceProviders: map[string]*string{
				"Applications.Core": to.Ptr("https://applications-rp"),
			},
		},
	}

	expectedPlaneList := &v1.PaginatedList{
		Value: []any{
			&expectedPlane,
		},
	}

	require.IsType(t, &armrpc_rest.OKResponse{}, actualResponse)
	actualBody := actualResponse.(*armrpc_rest.OKResponse).Body
	require.IsType(t, &v1.PaginatedList{}, actualBody)
	actualList := actualBody.(*v1.PaginatedList)

	// SystemData includes timestamps, so blank it out for comparison
	for i := range actualList.Value {
		require.IsType(t, &v20231001preview.RadiusPlaneResource{}, actualList.Value[i])
		actualList.Value[i].(*v20231001preview.RadiusPlaneResource).SystemData = nil
	}

	expectedResponse := armrpc_rest.NewOKResponse(expectedPlaneList)
	require.Equal(t, expectedResponse, actualResponse)
}
