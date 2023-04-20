// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestDeleteResourceRun_20230415preview(t *testing.T) {

	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *deployment.MockDeploymentProcessor, *ctrl.Request) {
		mctrl := gomock.NewController(t)

		msc := store.NewMockStorageClient(mctrl)
		mdp := deployment.NewMockDeploymentProcessor(mctrl)

		req := &ctrl.Request{
			OperationID:      uuid.New(),
			OperationType:    "APPLICATIONS.LINK/MONGODATABASES|DELETE",
			ResourceID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/mongoDatabases/mongo0",
			CorrelationID:    uuid.NewString(),
			OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
		}

		return func(tb testing.TB) {
			mctrl.Finish()
		}, msc, mdp, req
	}

	t.Parallel()

	deleteCases := []struct {
		desc     string
		getErr   error
		dpDelErr error
		scDelErr error
	}{
		{"delete-existing-resource", nil, nil, nil},
		{"delete-non-existing-resource", &store.ErrNotFound{}, nil, nil},
		{"delete-resource-dp-delete-error", nil, errors.New("deployment processor delete error"), nil},
		{"delete-resource-delete-from-db-error", nil, nil, errors.New("delete from db error")},
	}

	for _, tt := range deleteCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, msc, mdp, req := setupTest(t)
			defer teardownTest(t)

			msc.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{}, tt.getErr).
				Times(1)

			if tt.getErr == nil {
				mdp.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(tt.dpDelErr).
					Times(1)

				if tt.dpDelErr == nil {
					msc.EXPECT().
						Delete(gomock.Any(), gomock.Any()).
						Return(tt.scDelErr).
						Times(1)
				}
			}

			opts := ctrl.Options{
				StorageClient: msc,
				GetLinkDeploymentProcessor: func() deployment.DeploymentProcessor {
					return mdp
				},
			}

			ctrl, err := NewDeleteResource(opts)
			require.NoError(t, err)

			_, err = ctrl.Run(context.Background(), req)

			if tt.getErr != nil || tt.dpDelErr != nil || tt.scDelErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeleteResourceRunInvalidResourceType_20230415preview(t *testing.T) {

	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *deployment.MockDeploymentProcessor, *ctrl.Request) {
		mctrl := gomock.NewController(t)

		msc := store.NewMockStorageClient(mctrl)
		mdp := deployment.NewMockDeploymentProcessor(mctrl)

		req := &ctrl.Request{
			OperationID:      uuid.New(),
			OperationType:    "APPLICATIONS.LINK/INVALID|DELETE",
			ResourceID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/invalidType/invalid",
			CorrelationID:    uuid.NewString(),
			OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
		}

		return func(tb testing.TB) {
			mctrl.Finish()
		}, msc, mdp, req
	}

	t.Parallel()

	t.Run("deleting-invalid-resource", func(t *testing.T) {
		teardownTest, msc, mdp, req := setupTest(t)
		defer teardownTest(t)

		msc.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(&store.Object{}, nil).
			Times(1)
		opts := ctrl.Options{
			StorageClient: msc,
			GetLinkDeploymentProcessor: func() deployment.DeploymentProcessor {
				return mdp
			},
		}

		ctrl, err := NewDeleteResource(opts)
		require.NoError(t, err)

		_, err = ctrl.Run(context.Background(), req)
		require.Error(t, err)
		require.Equal(t, "async delete operation unsupported on resource type: \"applications.link/invalidtype\". Resource ID: \"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/invalidType/invalid\"", err.Error())
	})
}
