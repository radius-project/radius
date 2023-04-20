// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	http "net/http"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func Test_ListPlanes(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockStorageClient := store.NewMockStorageClient(mockCtrl)

	planesCtrl, err := NewListPlanes(ctrl.Options{
		Options: armrpc_controller.Options{
			StorageClient: mockStorageClient,
		},
	})
	require.NoError(t, err)

	url := "/planes?api-version=2023-04-15-preview"

	query := store.Query{
		RootScope:    "/planes",
		IsScopeQuery: true,
	}

	testPlaneId := "/planes/radius"
	testPlaneName := "radius"
	testPlaneType := "planes"

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
	ctx := testutil.ARMTestContextFromRequest(request)
	actualResponse, err := planesCtrl.Run(ctx, nil, request)
	require.NoError(t, err)

	expectedPlane := v20230415preview.PlaneResource{
		ID:   &testPlaneId,
		Name: &testPlaneName,
		Type: &testPlaneType,
		Tags: nil,
		Properties: &v20230415preview.PlaneResourceProperties{
			Kind:              to.Ptr(v20230415preview.PlaneKindAWS),
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
