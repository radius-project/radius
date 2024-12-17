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

package rediscaches

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/datastoresrp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/portableresources/renderers"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestListSecrets_20231001Preview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	ctx := context.Background()

	_, redisDataModel, _ := getTestModels20231001preview()

	t.Run("listSecrets non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)

		databaseClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
				return nil, &database.ErrNotFound{ID: id}
			})

		opts := ctrl.Options{
			DatabaseClient: databaseClient,
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
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)
		expectedSecrets := map[string]any{
			renderers.ConnectionURIValue:    "test-connection-uri",
			renderers.PasswordStringHolder:  "testPassword",
			renderers.ConnectionStringValue: "test-connection-string",
		}

		databaseClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
				return &database.Object{
					Metadata: database.Metadata{ID: id},
					Data:     redisDataModel,
				}, nil
			})

		opts := ctrl.Options{
			DatabaseClient: databaseClient,
		}

		ctl, err := NewListSecretsRedisCache(opts)
		require.NoError(t, err)

		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20231001preview.RedisCacheSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[renderers.ConnectionStringValue], *actualOutput.ConnectionString)
		require.Equal(t, expectedSecrets[renderers.PasswordStringHolder], *actualOutput.Password)
	})

	t.Run("listSecrets existing resource partial secrets", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)
		expectedSecrets := map[string]any{
			renderers.PasswordStringHolder: "testPassword",
		}

		databaseClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
				return &database.Object{
					Metadata: database.Metadata{ID: id},
					Data:     redisDataModel,
				}, nil
			})

		opts := ctrl.Options{
			DatabaseClient: databaseClient,
		}

		ctl, err := NewListSecretsRedisCache(opts)
		require.NoError(t, err)

		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20231001preview.RedisCacheSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[renderers.PasswordStringHolder], *actualOutput.Password)
	})

	t.Run("listSecrets error retrieving resource", func(t *testing.T) {
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)
		w := httptest.NewRecorder()

		databaseClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
				return nil, errors.New("failed to get the resource from data store")
			})

		opts := ctrl.Options{
			DatabaseClient: databaseClient,
		}

		ctl, err := NewListSecretsRedisCache(opts)
		require.NoError(t, err)

		_, err = ctl.Run(ctx, w, req)
		require.Error(t, err)
	})

	t.Run("listSecrets error invalid api-version", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)
		sCtx := v1.ARMRequestContextFromContext(ctx)
		sCtx.APIVersion = "invalid-api-version"

		databaseClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
				return &database.Object{
					Metadata: database.Metadata{ID: id},
					Data:     redisDataModel,
				}, nil
			})

		opts := ctrl.Options{
			DatabaseClient: databaseClient,
		}

		ctl, err := NewListSecretsRedisCache(opts)
		require.NoError(t, err)

		resp, err := ctl.Run(ctx, w, req)
		require.Error(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 400, w.Result().StatusCode)
	})

}
