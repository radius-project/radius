/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

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
	deployment "github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/linkrp"
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
			"container-put-success",
			container.ResourceType,
			"APPLICATIONS.CORE/CONTAINERS|PUT",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/%s", uuid.NewString()),
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"container-put-not-found",
			container.ResourceType,
			"APPLICATIONS.CORE/CONTAINERS|PUT",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/%s", uuid.NewString()),
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"container-put-get-err",
			container.ResourceType,
			"APPLICATIONS.CORE/CONTAINERS|PUT",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/%s", uuid.NewString()),
			errors.New("error getting object"),
			false,
			nil,
			nil,
			nil,
			errors.New("error getting object"),
		},
		{
			"http-route-put-success",
			httproute.ResourceType,
			"APPLICATIONS.CORE/HTTPROUTES|PUT",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/httpRoutes/%s", uuid.NewString()),
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"http-route-put-not-found",
			httproute.ResourceType,
			"APPLICATIONS.CORE/HTTPROUTES|PUT",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/httpRoutes/%s", uuid.NewString()),
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"gateway-put-success",
			gateway.ResourceType,
			"APPLICATIONS.CORE/GATEWAYS|PUT",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/%s", uuid.NewString()),
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"gateway-put-not-found",
			gateway.ResourceType,
			"APPLICATIONS.CORE/GATEWAYS|PUT",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/%s", uuid.NewString()),
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"unsupported-type-put",
			linkrp.RedisCachesResourceType,
			"APPLICATIONS.LINK/REDISCACHES|PUT",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/rc0",
			nil,
			true,
			nil,
			nil,
			nil,
			nil,
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
				GetDeploymentProcessor: func() deployment.DeploymentProcessor {
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
			"container-patch-success",
			container.ResourceType,
			"APPLICATIONS.CORE/CONTAINERS|PATCH",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/%s", uuid.NewString()),
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"container-patch-not-found",
			container.ResourceType,
			"APPLICATIONS.CORE/CONTAINERS|PATCH",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/%s", uuid.NewString()),
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			&store.ErrNotFound{},
		},
		{
			"container-patch-get-err",
			container.ResourceType,
			"APPLICATIONS.CORE/CONTAINERS|PATCH",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/%s", uuid.NewString()),
			errors.New("error getting object"),
			false,
			nil,
			nil,
			nil,
			errors.New("error getting object"),
		},
		{
			"http-route-patch-success",
			httproute.ResourceType,
			"APPLICATIONS.CORE/HTTPROUTES|PATCH",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/httpRoutes/%s", uuid.NewString()),
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"http-route-patch-not-found",
			httproute.ResourceType,
			"APPLICATIONS.CORE/HTTPROUTES|PATCH",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/httpRoutes/%s", uuid.NewString()),
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			&store.ErrNotFound{},
		},
		{
			"gateway-patch-success",
			gateway.ResourceType,
			"APPLICATIONS.CORE/GATEWAYS|PATCH",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/%s", uuid.NewString()),
			nil,
			false,
			nil,
			nil,
			nil,
			nil,
		},
		{
			"gateway-patch-not-found",
			gateway.ResourceType,
			"APPLICATIONS.CORE/GATEWAYS|PATCH",
			fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/gateways/%s", uuid.NewString()),
			&store.ErrNotFound{},
			false,
			nil,
			nil,
			nil,
			&store.ErrNotFound{},
		},
		{
			"unsupported-type-patch",
			linkrp.RedisCachesResourceType,
			"APPLICATIONS.LINK/REDISCACHES|PATCH",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/rc0",
			nil,
			true,
			nil,
			nil,
			nil,
			nil,
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
				GetDeploymentProcessor: func() deployment.DeploymentProcessor {
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
