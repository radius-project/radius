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

	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func Test_ListPlanesByType(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewListPlanesByType(armrpc_controller.Options{StorageClient: mockStorageClient})
	require.NoError(t, err)

	url := "/planes/radius?api-version=2022-09-01-privatepreview"

	query := store.Query{
		RootScope:    "/planes",
		IsScopeQuery: true,
		ResourceType: "radius",
	}

	testPlaneId := "/planes/radius/local"
	testPlaneName := "local"
	testPlaneType := "planesType"

	planeData := datamodel.Plane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   testPlaneId,
				Name: testPlaneName,
				Type: testPlaneType,
			},
		},
		Properties: datamodel.PlaneProperties{
			Kind: "AWS",
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
	actualResponse, err := planesCtrl.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedPlane := v20220901privatepreview.PlaneResource{
		ID:   &testPlaneId,
		Name: &testPlaneName,
		Type: &testPlaneType,
		Tags: nil,
		Properties: &v20220901privatepreview.PlaneResourceProperties{
			Kind:              to.Ptr(v20220901privatepreview.PlaneKindAWS),
			ResourceProviders: nil,
			URL:               nil,
			ProvisioningState: nil,
		},
	}

	expectedPlaneList := &v1.PaginatedList{
		Value: []any{
			&expectedPlane,
		},
	}

	expectedResponse := armrpc_rest.NewOKResponse(expectedPlaneList)

	require.Equal(t, expectedResponse, actualResponse)
}
