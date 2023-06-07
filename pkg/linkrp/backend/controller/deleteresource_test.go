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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/linkrp/model"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

var outputResourceResourceID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.DocumentDB/databaseAccounts/mongoDatabases"
var outputResource = rpv1.OutputResource{
	Identity: resourcemodel.NewARMIdentity(&resourcemodel.ResourceType{
		Type:     "Microsoft.DocumentDB/databaseAccounts/mongoDatabases",
		Provider: resourcemodel.ProviderAzure,
	}, outputResourceResourceID, "2022-01-01"),
}

func TestDeleteResourceRun_20220315PrivatePreview(t *testing.T) {

	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *processors.MockResourceClient, *model.ApplicationModel, *ctrl.Request) {
		mctrl := gomock.NewController(t)

		msc := store.NewMockStorageClient(mctrl)
		client := processors.NewMockResourceClient(mctrl)

		req := &ctrl.Request{
			OperationID:      uuid.New(),
			OperationType:    "APPLICATIONS.LINK/MONGODATABASES|DELETE",
			ResourceID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/mongoDatabases/mongo0",
			CorrelationID:    uuid.NewString(),
			OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
		}

		model := model.NewModel(
			model.RecipeModel{},
			[]model.RadiusResourceModel{},
			[]model.OutputResourceModel{
				{
					// Handles all AWS types
					ResourceType: resourcemodel.ResourceType{
						Type:     "",
						Provider: "",
					},
				},
			},
			map[string]bool{
				resourcemodel.ProviderKubernetes: true,
				resourcemodel.ProviderAzure:      true,
				resourcemodel.ProviderAWS:        true,
			})

		return func(tb testing.TB) {
			mctrl.Finish()
		}, msc, client, &model, req
	}

	t.Parallel()

	deleteCases := []struct {
		desc         string
		getErr       error
		clientDelErr error
		scDelErr     error
	}{
		{"delete-existing-resource", nil, nil, nil},
		{"delete-non-existing-resource", &store.ErrNotFound{}, nil, nil},
		{"delete-resource-client-delete-error", nil, errors.New("resource client delete error"), nil},
		{"delete-resource-delete-from-db-error", nil, nil, errors.New("delete from db error")},
	}

	for _, tt := range deleteCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, msc, client, model, req := setupTest(t)
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

			msc.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{Data: data}, tt.getErr).
				Times(1)

			if tt.getErr == nil {
				client.EXPECT().
					Delete(gomock.Any(), outputResourceResourceID, resourcemodel.APIVersionUnknown).
					Return(tt.clientDelErr).
					Times(1)

				if tt.clientDelErr == nil {
					msc.EXPECT().
						Delete(gomock.Any(), gomock.Any()).
						Return(tt.scDelErr).
						Times(1)
				}
			}

			opts := ctrl.Options{
				StorageClient: msc,
			}

			ctrl, err := NewDeleteResource(opts, client, *model)
			require.NoError(t, err)

			_, err = ctrl.Run(context.Background(), req)

			if tt.getErr != nil || tt.clientDelErr != nil || tt.scDelErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeleteResourceRunInvalidResourceType_20220315PrivatePreview(t *testing.T) {

	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *processors.MockResourceClient, *model.ApplicationModel, *ctrl.Request) {
		mctrl := gomock.NewController(t)

		msc := store.NewMockStorageClient(mctrl)
		client := processors.NewMockResourceClient(mctrl)

		model := model.NewModel(
			model.RecipeModel{},
			[]model.RadiusResourceModel{},
			[]model.OutputResourceModel{
				{
					// Handles all AWS types
					ResourceType: resourcemodel.ResourceType{
						Type:     "",
						Provider: "",
					},
				},
			},
			map[string]bool{
				resourcemodel.ProviderKubernetes: true,
				resourcemodel.ProviderAzure:      true,
				resourcemodel.ProviderAWS:        true,
			})

		req := &ctrl.Request{
			OperationID:      uuid.New(),
			OperationType:    "APPLICATIONS.LINK/INVALID|DELETE",
			ResourceID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/invalidType/invalid",
			CorrelationID:    uuid.NewString(),
			OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
		}

		return func(tb testing.TB) {
			mctrl.Finish()
		}, msc, client, &model, req
	}

	t.Parallel()

	t.Run("deleting-invalid-resource", func(t *testing.T) {
		teardownTest, msc, client, model, req := setupTest(t)
		defer teardownTest(t)

		msc.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(&store.Object{}, nil).
			Times(1)
		opts := ctrl.Options{
			StorageClient: msc,
		}

		ctrl, err := NewDeleteResource(opts, client, *model)
		require.NoError(t, err)

		_, err = ctrl.Run(context.Background(), req)
		require.Error(t, err)
		require.Equal(t, "async delete operation unsupported on resource type: \"applications.link/invalidtype\". Resource ID: \"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/invalidType/invalid\"", err.Error())
	})
}
