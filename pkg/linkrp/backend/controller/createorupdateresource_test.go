// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

func TestCreateOrUpdateResourceRun_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *deployment.MockDeploymentProcessor) {
		mctrl := gomock.NewController(t)

		msc := store.NewMockStorageClient(mctrl)
		mdp := deployment.NewMockDeploymentProcessor(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, msc, mdp
	}

	putCases := []struct {
		desc      string
		rt        string
		opType    string
		rId       string
		getErr    error
		convErr   bool
		renderErr error
		deployErr error
		saveErr   error
		expErr    error
	}{
		{
			"mongo-put-success",
			linkrp.MongoDatabasesResourceType,
			"APPLICATIONS.LINK/MONGODATABASES|PUT",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/mongoDatabases/mongo0",
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"mongo-put-not-found",
			linkrp.MongoDatabasesResourceType,
			"APPLICATIONS.LINK/MONGODATABASES|PUT",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/mongoDatabases/mongo1",
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"mongo-put-get-err",
			linkrp.MongoDatabasesResourceType,
			"APPLICATIONS.LINK/MONGODATABASES|PUT",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/mongoDatabases/mongo2",
			errors.New("error getting object"),
			false,
			nil,
			nil,
			nil,
			errors.New("error getting object"),
		},
		{
			"redis-put-success",
			linkrp.RedisCachesResourceType,
			"APPLICATIONS.LINK/REDISCACHES|PUT",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/redis0",
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"redis-put-not-found",
			linkrp.RedisCachesResourceType,
			"APPLICATIONS.LINK/REDISCACHES|PUT",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/redis1",
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"redis-put-get-err",
			linkrp.RedisCachesResourceType,
			"APPLICATIONS.LINK/REDISCACHES|PUT",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/redis2",
			errors.New("error getting object"),
			false,
			nil,
			nil,
			nil,
			errors.New("error getting object"),
		},
	}

	for _, tt := range putCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, msc, mdp := setupTest(t)
			defer teardownTest(t)

			req := &ctrl.Request{
				OperationID:      uuid.New(),
				OperationType:    tt.opType,
				ResourceID:       tt.rId,
				CorrelationID:    uuid.NewString(),
				OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
			}

			// This code is general and we might be processing an async job for a resource or a scope, so using the general Parse function.
			parsedID, err := resources.Parse(tt.rId)
			require.NoError(t, err)

			getCall := msc.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{
					Data: map[string]any{
						"name": "env0",
						"properties": map[string]any{
							"provisioningState": "Accepted",
						},
					},
				}, tt.getErr).
				Times(1)

			if (tt.getErr == nil || errors.Is(&store.ErrNotFound{}, tt.getErr)) && !tt.convErr {
				renderCall := mdp.EXPECT().
					Render(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(renderers.RendererOutput{}, tt.renderErr).
					After(getCall).
					Times(1)

				if tt.renderErr == nil {
					deployCall := mdp.EXPECT().
						Deploy(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(rpv1.DeploymentOutput{}, tt.deployErr).
						After(renderCall).
						Times(1)

					if !errors.Is(&store.ErrNotFound{}, tt.getErr) {
						mdp.EXPECT().
							Delete(gomock.Any(), gomock.Any(), gomock.Any()).
							Return(nil).
							After(deployCall).
							Times(1)
					}

					if tt.deployErr == nil {
						msc.EXPECT().
							Save(gomock.Any(), gomock.Any(), gomock.Any()).
							Return(tt.saveErr).
							After(deployCall).
							Times(1)
					}
				}
			}

			opts := ctrl.Options{
				StorageClient: msc,
				GetLinkDeploymentProcessor: func() deployment.DeploymentProcessor {
					return mdp
				},
			}

			genCtrl, err := NewCreateOrUpdateResource(opts)
			require.NoError(t, err)

			res, err := genCtrl.Run(context.Background(), req)

			if tt.convErr {
				tt.expErr = fmt.Errorf("invalid resource type: %q for dependent resource ID: %q", strings.ToLower(tt.rt), parsedID.String())
			}

			if tt.expErr != nil {
				require.Error(t, err)
				require.Equal(t, tt.expErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, ctrl.Result{}, res)
			}
		})
	}
	patchCases := []struct {
		desc      string
		rt        string
		opType    string
		rId       string
		getErr    error
		convErr   bool
		renderErr error
		deployErr error
		saveErr   error
		expErr    error
	}{
		{
			"mongo-patch-success",
			linkrp.MongoDatabasesResourceType,
			"APPLICATIONS.LINK/MONGODATABASES|PATCH",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/mongoDatabases/mongo0",
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"mongo-patch-not-found",
			linkrp.MongoDatabasesResourceType,
			"APPLICATIONS.LINK/MONGODATABASES|PATCH",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/mongoDatabases/mongo1",
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			&store.ErrNotFound{},
		},
		{
			"mongo-patch-get-err",
			linkrp.MongoDatabasesResourceType,
			"APPLICATIONS.LINK/MONGODATABASES|PATCH",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/mongoDatabases/mongo2",
			errors.New("error getting object"),
			false,
			nil,
			nil,
			nil,
			errors.New("error getting object"),
		},
		{
			"redis-patch-success",
			linkrp.RedisCachesResourceType,
			"APPLICATIONS.LINK/REDISCACHES|PATCH",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/redis0",
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"redis-patch-not-found",
			linkrp.RedisCachesResourceType,
			"APPLICATIONS.LINK/REDISCACHES|PATCH",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/redis1",
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			&store.ErrNotFound{},
		},
		{
			"redis-patch-get-err",
			linkrp.RedisCachesResourceType,
			"APPLICATIONS.LINK/REDISCACHES|PATCH",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/redis2",
			errors.New("error getting object"),
			false,
			nil,
			nil,
			nil,
			errors.New("error getting object"),
		},
	}

	for _, tt := range patchCases {
		t.Run(tt.desc, func(t *testing.T) {
			teardownTest, msc, mdp := setupTest(t)
			defer teardownTest(t)

			req := &ctrl.Request{
				OperationID:      uuid.New(),
				OperationType:    tt.opType,
				ResourceID:       tt.rId,
				CorrelationID:    uuid.NewString(),
				OperationTimeout: &ctrl.DefaultAsyncOperationTimeout,
			}

			// This code is general and we might be processing an async job for a resource or a scope, so using the general Parse function.
			parsedID, err := resources.Parse(tt.rId)
			require.NoError(t, err)

			getCall := msc.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				Return(&store.Object{
					Data: map[string]any{
						"name": "env0",
						"properties": map[string]any{
							"provisioningState": "Accepted",
						},
					},
				}, tt.getErr).
				Times(1)

			if tt.getErr == nil && !tt.convErr {
				renderCall := mdp.EXPECT().
					Render(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(renderers.RendererOutput{}, tt.renderErr).
					After(getCall).
					Times(1)

				if tt.renderErr == nil {
					deployCall := mdp.EXPECT().
						Deploy(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(rpv1.DeploymentOutput{}, tt.deployErr).
						After(renderCall).
						Times(1)

					deleteCall := mdp.EXPECT().
						Delete(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(nil).
						After(deployCall).
						Times(1)

					if tt.deployErr == nil {
						msc.EXPECT().
							Save(gomock.Any(), gomock.Any(), gomock.Any()).
							Return(tt.saveErr).
							After(deployCall).
							After(deleteCall).
							Times(1)
					}
				}
			}
			opts := ctrl.Options{
				StorageClient: msc,
				GetLinkDeploymentProcessor: func() deployment.DeploymentProcessor {
					return mdp
				},
			}

			genCtrl, err := NewCreateOrUpdateResource(opts)
			require.NoError(t, err)

			res, err := genCtrl.Run(context.Background(), req)

			if tt.convErr {
				tt.expErr = fmt.Errorf("invalid resource type: %q for dependent resource ID: %q", strings.ToLower(tt.rt), parsedID.String())
			}

			if tt.expErr != nil {
				require.Error(t, err)
				require.Equal(t, tt.expErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, ctrl.Result{}, res)
			}
		})
	}
}
