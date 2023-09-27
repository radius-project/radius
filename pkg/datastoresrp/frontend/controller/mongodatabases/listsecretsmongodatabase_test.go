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

package mongodatabases

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
	"github.com/radius-project/radius/pkg/datastoresrp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/portableresources/renderers"
	"github.com/radius-project/radius/pkg/ucp/store"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestListSecrets_20231001Preview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	_, mongoDataModel, _ := getTestModels20231001preview()

	t.Run("listSecrets non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{ID: id}
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewListSecretsMongoDatabase(opts)

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
			renderers.UsernameStringValue:   "testUser",
			renderers.PasswordStringHolder:  "testPassword",
			renderers.ConnectionStringValue: "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255",
		}

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     mongoDataModel,
				}, nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewListSecretsMongoDatabase(opts)
		require.NoError(t, err)

		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20231001preview.MongoDatabaseSecrets{}
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
			renderers.UsernameStringValue:   "testUser",
			renderers.ConnectionStringValue: "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255",
		}

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     mongoDataModel,
				}, nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewListSecretsMongoDatabase(opts)
		require.NoError(t, err)

		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20231001preview.MongoDatabaseSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[renderers.ConnectionStringValue], *actualOutput.ConnectionString)
	})

	t.Run("listSecrets error retrieving resource", func(t *testing.T) {
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)
		w := httptest.NewRecorder()

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, errors.New("failed to get the resource from data store")
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewListSecretsMongoDatabase(opts)
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

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     mongoDataModel,
				}, nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewListSecretsMongoDatabase(opts)
		require.NoError(t, err)

		resp, err := ctl.Run(ctx, w, req)
		require.Error(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 400, w.Result().StatusCode)
	})
}
