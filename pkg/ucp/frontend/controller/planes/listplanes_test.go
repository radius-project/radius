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
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_ListPlanes(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockDatabaseClient := database.NewMockClient(mockCtrl)

	planesCtrl, err := NewListPlanes(armrpc_controller.Options{DatabaseClient: mockDatabaseClient})
	require.NoError(t, err)

	url := "/planes?api-version=2023-10-01-preview"

	testPlaneId := "/planes/aws"
	testPlaneName := "aws"
	testPlaneType := datamodel.AWSPlaneResourceType

	planeData := datamodel.AWSPlane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       testPlaneId,
				Name:     testPlaneName,
				Type:     testPlaneType,
				Location: "global",
			},
		},
		Properties: datamodel.AWSPlaneProperties{},
	}

	mockDatabaseClient.EXPECT().Query(gomock.Any(), database.Query{
		RootScope:    "/planes",
		ResourceType: "aws",
		IsScopeQuery: true,
	}).Return(&database.ObjectQueryResult{
		Items: []database.Object{
			{
				Metadata: database.Metadata{},
				Data:     &planeData,
			},
		},
	}, nil)

	mockDatabaseClient.EXPECT().Query(gomock.Any(), database.Query{
		RootScope:    "/planes",
		ResourceType: "azure",
		IsScopeQuery: true,
	}).Return(&database.ObjectQueryResult{}, nil)

	mockDatabaseClient.EXPECT().Query(gomock.Any(), database.Query{
		RootScope:    "/planes",
		ResourceType: "radius",
		IsScopeQuery: true,
	}).Return(&database.ObjectQueryResult{}, nil)

	request, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(request)
	actualResponse, err := planesCtrl.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedPlane := v20231001preview.GenericPlaneResource{
		ID:       &testPlaneId,
		Name:     &testPlaneName,
		Type:     &testPlaneType,
		Location: to.Ptr("global"),
		Tags:     map[string]*string{},
		Properties: &v20231001preview.GenericPlaneResourceProperties{
			ProvisioningState: to.Ptr(v20231001preview.ProvisioningState("Succeeded")),
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
		require.IsType(t, &v20231001preview.GenericPlaneResource{}, actualList.Value[i])
		actualList.Value[i].(*v20231001preview.GenericPlaneResource).SystemData = nil
	}

	expectedResponse := armrpc_rest.NewOKResponse(expectedPlaneList)
	require.Equal(t, expectedResponse, actualResponse)
}
