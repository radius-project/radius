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

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/engine"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

var outputResourceResourceID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.DocumentDB/databaseAccounts/mongoDatabases"
var outputResource = rpv1.OutputResource{
	ID:            resources.MustParse(outputResourceResourceID),
	RadiusManaged: to.Ptr(true),
}

func TestDeleteResourceRun_20220315PrivatePreview(t *testing.T) {
	resourceID := "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/mongoDatabases/mongo0"
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *ctrl.Request, *engine.MockEngine) {
		mctrl := gomock.NewController(t)

		msc := store.NewMockStorageClient(mctrl)
		eng := engine.NewMockEngine(mctrl)

		req := &ctrl.Request{
			OperationID:      uuid.New(),
			OperationType:    "APPLICATIONS.DATASTORES/MONGODATABASES|DELETE",
			ResourceID:       resourceID,
			CorrelationID:    uuid.NewString(),
			OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
		}

		return func(tb testing.TB) {
			mctrl.Finish()
		}, msc, req, eng
	}

	t.Parallel()

	deleteCases := []struct {
		desc      string
		getErr    error
		engDelErr error
		scDelErr  error
	}{
		{"delete-existing-resource", nil, nil, nil},
		{"delete-non-existing-resource", &store.ErrNotFound{ID: resourceID}, nil, nil},
		{"delete-resource-engine-delete-error", nil, errors.New("engine delete error"), nil},
		{"delete-resource-delete-from-db-error", nil, nil, errors.New("delete from db error")},
	}

	for _, tt := range deleteCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, msc, req, eng := setupTest(t)
			defer teardownTest(t)

			status := rpv1.ResourceStatus{
				OutputResources: []rpv1.OutputResource{
					outputResource,
				},
			}
			sb, err := json.Marshal(&status)
			require.NoError(t, err)

			sm := map[string]interface{}{}
			err = json.Unmarshal(sb, &sm)
			require.NoError(t, err)

			data := map[string]any{
				"name":     "tr",
				"type":     "Applications.Test/testResources",
				"id":       TestResourceID,
				"location": v1.LocationGlobal,
				"properties": map[string]any{
					"application":       TestApplicationID,
					"environment":       TestEnvironmentID,
					"provisioningState": "Accepted",
					"status":            sm,
				},
			}

			recipeData := recipes.ResourceMetadata{
				Name:          "",
				EnvironmentID: TestEnvironmentID,
				ApplicationID: TestApplicationID,
				Parameters:    nil,
				ResourceID:    resourceID,
			}

			msc.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{Data: data}, tt.getErr).
				Times(1)

			if tt.getErr == nil {
				eng.EXPECT().
					Delete(gomock.Any(), engine.DeleteOptions{
						BaseOptions: engine.BaseOptions{
							Recipe: recipeData,
						},
						OutputResources: status.OutputResources,
					}).
					Return(tt.engDelErr).
					Times(1)
				if tt.engDelErr == nil {
					msc.EXPECT().
						Delete(gomock.Any(), gomock.Any()).
						Return(tt.scDelErr).
						Times(1)
				}
			}
			opts := ctrl.Options{
				StorageClient: msc,
			}

			ctrl, err := NewDeleteResource(opts, eng)
			require.NoError(t, err)

			_, err = ctrl.Run(context.Background(), req)

			if tt.getErr != nil || tt.engDelErr != nil || tt.scDelErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeleteResourceRunInvalidResourceType_20220315PrivatePreview(t *testing.T) {

	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *ctrl.Request, *gomock.Controller) {
		mctrl := gomock.NewController(t)

		msc := store.NewMockStorageClient(mctrl)

		req := &ctrl.Request{
			OperationID:      uuid.New(),
			OperationType:    "APPLICATIONS.DAPR/INVALID|DELETE",
			ResourceID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/invalidType/invalid",
			CorrelationID:    uuid.NewString(),
			OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
		}

		return func(tb testing.TB) {
			mctrl.Finish()
		}, msc, req, mctrl
	}

	t.Parallel()

	t.Run("deleting-invalid-resource", func(t *testing.T) {
		teardownTest, msc, req, mctrl := setupTest(t)
		defer teardownTest(t)

		msc.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(&store.Object{}, nil).
			Times(1)
		opts := ctrl.Options{
			StorageClient: msc,
		}

		eng := engine.NewMockEngine(mctrl)
		ctrl, err := NewDeleteResource(opts, eng)
		require.NoError(t, err)

		_, err = ctrl.Run(context.Background(), req)
		require.Error(t, err)
		require.Equal(t, "async delete operation unsupported on resource type: \"applications.dapr/invalidtype\". Resource ID: \"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/invalidType/invalid\"", err.Error())
	})
}
