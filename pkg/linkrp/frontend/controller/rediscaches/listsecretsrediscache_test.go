// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

func TestListSecrets_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)
	ctx := context.Background()

	_, redisDataModel, _ := getTestModels20220315privatepreview()

	t.Run("listSecrets non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return mDeploymentProcessor
			},
		}

		ctl, err := NewListSecretsRedisCache(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
	})

	t.Run("listSecrets existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)
		expectedSecrets := map[string]any{
			renderers.PasswordStringHolder:  "testPassword",
			renderers.ConnectionStringValue: "test-connection-string",
		}

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     redisDataModel,
				}, nil
			})
		mDeploymentProcessor.EXPECT().FetchSecrets(gomock.Any(), gomock.Any()).Times(1).Return(expectedSecrets, nil)

		opts := ctrl.Options{
			StorageClient: mStorageClient,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return mDeploymentProcessor
			},
		}

		ctl, err := NewListSecretsRedisCache(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.RedisCacheSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[renderers.ConnectionStringValue], *actualOutput.ConnectionString)
		require.Equal(t, expectedSecrets[renderers.PasswordStringHolder], *actualOutput.Password)
	})

	t.Run("listSecrets existing resource partial secrets", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)
		expectedSecrets := map[string]any{
			renderers.PasswordStringHolder: "testPassword",
		}

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     redisDataModel,
				}, nil
			})
		mDeploymentProcessor.EXPECT().FetchSecrets(gomock.Any(), gomock.Any()).Times(1).Return(expectedSecrets, nil)

		opts := ctrl.Options{
			StorageClient: mStorageClient,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return mDeploymentProcessor
			},
		}

		ctl, err := NewListSecretsRedisCache(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.RedisCacheSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[renderers.PasswordStringHolder], *actualOutput.Password)
	})

	t.Run("listSecrets error retrieving resource", func(t *testing.T) {
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)
		w := httptest.NewRecorder()

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, errors.New("failed to get the resource from data store")
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return mDeploymentProcessor
			},
		}

		ctl, err := NewListSecretsRedisCache(opts)

		require.NoError(t, err)
		_, err = ctl.Run(ctx, w, req)
		require.Error(t, err)
	})

}
