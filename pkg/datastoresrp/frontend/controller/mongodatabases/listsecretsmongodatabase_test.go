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

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rpctest"
	"github.com/project-radius/radius/pkg/datastoresrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/ucp/store"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestListSecrets_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	_, mongoDataModel, _ := getTestModels20220315privatepreview()

	t.Run("listSecrets non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := rpctest.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
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
		req, err := rpctest.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.ARMTestContextFromRequest(req)
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

		actualOutput := &v20220315privatepreview.MongoDatabaseSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[renderers.ConnectionStringValue], *actualOutput.ConnectionString)
		require.Equal(t, expectedSecrets[renderers.UsernameStringValue], *actualOutput.Username)
		require.Equal(t, expectedSecrets[renderers.PasswordStringHolder], *actualOutput.Password)
	})

	t.Run("listSecrets existing resource partial secrets", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := rpctest.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.ARMTestContextFromRequest(req)
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

		actualOutput := &v20220315privatepreview.MongoDatabaseSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[renderers.UsernameStringValue], *actualOutput.Username)
		require.Equal(t, expectedSecrets[renderers.ConnectionStringValue], *actualOutput.ConnectionString)
	})

	t.Run("listSecrets error retrieving resource", func(t *testing.T) {
		req, err := rpctest.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		require.NoError(t, err)
		ctx := rpctest.ARMTestContextFromRequest(req)
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

}
