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

package rabbitmqqueues

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	frontend_ctrl "github.com/project-radius/radius/pkg/messagingrp/frontend/controller"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

// Test_ListSecrets_20220315PrivatePreview: These tests are for new resource Messaging.RabbitMQQueues
func Test_ListSecrets_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)
	ctx := context.Background()

	_, rabbitMQDataModel, _ := get_TestModel20220315privatepreview()

	t.Run("listSecrets non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		opts := frontend_ctrl.Options{
			Options: ctrl.Options{
				StorageClient: mStorageClient,
			},
			DeployProcessor: mDeploymentProcessor,
		}

		ctl, err := NewListSecretsRabbitMQQueue(opts)
		require.NoError(t, err)

		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
	})

	t.Run("listSecrets existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)
		expectedSecrets := map[string]any{
			renderers.ConnectionStringValue: "connection://string",
		}

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     rabbitMQDataModel,
				}, nil
			})
		mDeploymentProcessor.EXPECT().FetchSecrets(gomock.Any(), gomock.Any()).Times(1).Return(expectedSecrets, nil)

		opts := frontend_ctrl.Options{
			Options: ctrl.Options{
				StorageClient: mStorageClient,
			},
			DeployProcessor: mDeploymentProcessor,
		}

		ctl, err := NewListSecretsRabbitMQQueue(opts)
		require.NoError(t, err)

		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.RabbitMQSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[renderers.ConnectionStringValue], *actualOutput.ConnectionString)
	})

	t.Run("listSecrets existing resource empty secrets", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)
		expectedSecrets := map[string]any{}

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     rabbitMQDataModel,
				}, nil
			})
		mDeploymentProcessor.EXPECT().FetchSecrets(gomock.Any(), gomock.Any()).Times(1).Return(expectedSecrets, nil)

		opts := frontend_ctrl.Options{
			Options: ctrl.Options{
				StorageClient: mStorageClient,
			},
			DeployProcessor: mDeploymentProcessor,
		}

		ctl, err := NewListSecretsRabbitMQQueue(opts)
		require.NoError(t, err)

		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.RabbitMQSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, "", *actualOutput.ConnectionString)
	})

	t.Run("listSecrets error retrieving resource", func(t *testing.T) {
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)
		w := httptest.NewRecorder()

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, errors.New("failed to get the resource from data store")
			})

		opts := frontend_ctrl.Options{
			Options: ctrl.Options{
				StorageClient: mStorageClient,
			},
			DeployProcessor: mDeploymentProcessor,
		}

		ctl, err := NewListSecretsRabbitMQQueue(opts)
		require.NoError(t, err)

		_, err = ctl.Run(ctx, w, req)
		require.Error(t, err)
	})

}
