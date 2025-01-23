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

package resourcegroups

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/trackedresource"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_Run(t *testing.T) {
	setup := func(t *testing.T) (*TrackedResourceProcessController, *mockUpdater, *database.MockClient) {
		ctrl := gomock.NewController(t)
		databaseClient := database.NewMockClient(ctrl)

		pc, err := NewTrackedResourceProcessController(controller.Options{DatabaseClient: databaseClient}, nil, nil)
		require.NoError(t, err)

		updater := mockUpdater{}
		pc.(*TrackedResourceProcessController).updater = &updater
		return pc.(*TrackedResourceProcessController), &updater, databaseClient
	}

	id := resources.MustParse("/planes/test/local/resourceGroups/test-rg/providers/Applications.Test/testResources/my-resource")
	trackingID := trackedresource.IDFor(id)
	data := datamodel.GenericResourceFromID(id, trackingID)
	data.Properties.APIVersion = "2025-01-01"

	resourceTypeID, err := datamodel.ResourceTypeIDFromResourceID(id)
	require.NoError(t, err)

	locationID, err := datamodel.ResourceProviderLocationIDFromResourceID(id, "global")
	require.NoError(t, err)

	plane := datamodel.RadiusPlane{
		Properties: datamodel.RadiusPlaneProperties{
			ResourceProviders: map[string]string{
				"Applications.Test": "https://localhost:1234",
			},
		},
	}
	resourceGroup := &datamodel.ResourceGroup{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: id.RootScope(),
			},
		},
	}

	resourceTypeResource := &datamodel.ResourceType{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				Name: "testResources",
				ID:   resourceTypeID.String(),
			},
		},
		Properties: datamodel.ResourceTypeProperties{},
	}

	locationResource := &datamodel.Location{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				Name: "global",
				ID:   locationID.String(),
			},
		},
		Properties: datamodel.LocationProperties{
			Address: to.Ptr("https://localhost:1234"),
			ResourceTypes: map[string]datamodel.LocationResourceTypeConfiguration{
				"testResources": {
					APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{
						"2025-01-01": {},
					},
				},
			},
		},
	}

	// Most of the heavy lifting is done by the updater. We just need to test that we're calling it correctly.
	t.Run("Success", func(t *testing.T) {
		pc, _, databaseClient := setup(t)

		databaseClient.EXPECT().
			Get(gomock.Any(), trackingID.String(), gomock.Any()).
			Return(&database.Object{Data: data}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), trackingID.PlaneScope(), gomock.Any()).
			Return(&database.Object{Data: plane}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), resourceTypeID.String(), gomock.Any()).
			Return(&database.Object{Data: resourceTypeResource}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), trackingID.RootScope(), gomock.Any()).
			Return(&database.Object{Data: resourceGroup}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), locationResource.ID).
			Return(&database.Object{Data: locationResource}, nil).Times(1)

		result, err := pc.Run(testcontext.New(t), &controller.Request{ResourceID: trackingID.String()})
		require.Equal(t, controller.Result{}, result)
		require.NoError(t, err)
	})

	t.Run("retry", func(t *testing.T) {
		pc, updater, databaseClient := setup(t)

		databaseClient.EXPECT().
			Get(gomock.Any(), trackingID.String(), gomock.Any()).
			Return(&database.Object{Data: data}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), trackingID.PlaneScope(), gomock.Any()).
			Return(&database.Object{Data: plane}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), resourceTypeID.String(), gomock.Any()).
			Return(&database.Object{Data: resourceTypeResource}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), trackingID.RootScope(), gomock.Any()).
			Return(&database.Object{Data: resourceGroup}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), locationResource.ID).
			Return(&database.Object{Data: locationResource}, nil).Times(1)

		// Force a retry.
		updater.Result = &trackedresource.InProgressErr{}

		expected := controller.Result{}
		expected.SetFailed(v1.ErrorDetails{Code: v1.CodeConflict, Message: updater.Result.Error(), Target: trackingID.String()}, true)

		result, err := pc.Run(testcontext.New(t), &controller.Request{ResourceID: trackingID.String()})
		require.Equal(t, expected, result)
		require.NoError(t, err)
	})

	t.Run("Failure (resource not found)", func(t *testing.T) {
		pc, _, databaseClient := setup(t)

		databaseClient.EXPECT().
			Get(gomock.Any(), trackingID.String(), gomock.Any()).
			Return(nil, &database.ErrNotFound{}).Times(1)

		expected := controller.NewFailedResult(v1.ErrorDetails{
			Code:    v1.CodeNotFound,
			Message: fmt.Sprintf("resource %q not found", trackingID.String()),
			Target:  trackingID.String(),
		})

		result, err := pc.Run(testcontext.New(t), &controller.Request{ResourceID: trackingID.String()})
		require.Equal(t, expected, result)
		require.NoError(t, err)
	})

	t.Run("Failure (validate downstream: not found)", func(t *testing.T) {
		pc, _, databaseClient := setup(t)

		databaseClient.EXPECT().
			Get(gomock.Any(), trackingID.String(), gomock.Any()).
			Return(&database.Object{Data: data}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), "/planes/"+trackingID.PlaneNamespace(), gomock.Any()).
			Return(nil, &database.ErrNotFound{}).Times(1)

		expected := controller.NewFailedResult(v1.ErrorDetails{
			Code:    v1.CodeNotFound,
			Message: "plane \"/planes/test/local\" not found",
			Target:  trackingID.String(),
		})

		result, err := pc.Run(testcontext.New(t), &controller.Request{ResourceID: trackingID.String()})
		require.Equal(t, expected, result)
		require.NoError(t, err)
	})
}

type mockUpdater struct {
	Result error
}

func (u *mockUpdater) Update(ctx context.Context, downstreamURL string, originalID resources.ID, version string) error {
	return u.Result
}
