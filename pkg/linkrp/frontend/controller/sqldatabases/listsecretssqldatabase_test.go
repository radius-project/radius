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

package sqldatabases

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestListSecrets_20220315PrivatePreview(t *testing.T) {
	const (
		usernameStringValue   string = "username"
		passwordStringValue   string = "password"
		connectionStringValue string = "connectionString"
	)
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	_, sqlDataModel, _ := getTestModels20220315privatepreview()

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
		}

		ctl, err := NewListSecretsSqlDatabase(opts)

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
			passwordStringValue:   "testPassword",
			connectionStringValue: "Data Source=tcp:testAccount1.sql.cosmos.azure.com,1433;Initial Catalog=testDatabase;User Id=testUser;Password=testPassword;Encrypt=True;TrustServerCertificate=True",
		}

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     sqlDataModel,
				}, nil
			})

		opts := frontend_ctrl.Options{
			Options: ctrl.Options{
				StorageClient: mStorageClient,
			},
		}

		ctl, err := NewListSecretsSqlDatabase(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.SQLDatabaseSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[connectionStringValue], *actualOutput.ConnectionString)
		require.Equal(t, expectedSecrets[passwordStringValue], *actualOutput.Password)
	})

	t.Run("listSecrets existing resource partial secrets", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)
		expectedSecrets := map[string]any{
			connectionStringValue: "Data Source=tcp:testAccount1.sql.cosmos.azure.com,1433;Initial Catalog=testDatabase;User Id=testUser;Password=testPassword;Encrypt=True;TrustServerCertificate=True",
		}

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     sqlDataModel,
				}, nil
			})

		opts := frontend_ctrl.Options{
			Options: ctrl.Options{
				StorageClient: mStorageClient,
			},
		}

		ctl, err := NewListSecretsSqlDatabase(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.SQLDatabaseSecrets{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedSecrets[connectionStringValue], *actualOutput.ConnectionString)
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
		}

		ctl, err := NewListSecretsSqlDatabase(opts)

		require.NoError(t, err)
		_, err = ctl.Run(ctx, w, req)
		require.Error(t, err)
	})

}
