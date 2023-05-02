// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/datastoresrp/api/v20220315privatepreview"
	frontend_ctrl "github.com/project-radius/radius/pkg/datastoresrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestListSecrets_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)
	ctx := context.Background()

	_, mongoDataModel, _ := getTestModels20220315privatepreview()

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

		ctl, err := NewListSecretsMongoDatabase(opts)

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
			renderers.UsernameStringValue:   "testUser",
			renderers.PasswordStringHolder:  "testPassword",
			renderers.ConnectionStringValue: "testConnectionString",
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
		mDeploymentProcessor.EXPECT().FetchSecrets(gomock.Any(), gomock.Any()).Times(1).Return(expectedSecrets, nil)

		opts := frontend_ctrl.Options{
			Options: ctrl.Options{
				StorageClient: mStorageClient,
			},
			DeployProcessor: mDeploymentProcessor,
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
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)
		expectedSecrets := map[string]any{
			renderers.UsernameStringValue:   "testUser",
			renderers.ConnectionStringValue: "testConnectionString",
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
		mDeploymentProcessor.EXPECT().FetchSecrets(gomock.Any(), gomock.Any()).Times(1).Return(expectedSecrets, nil)

		opts := frontend_ctrl.Options{
			Options: ctrl.Options{
				StorageClient: mStorageClient,
			},
			DeployProcessor: mDeploymentProcessor,
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

		ctl, err := NewListSecretsMongoDatabase(opts)

		require.NoError(t, err)
		_, err = ctl.Run(ctx, w, req)
		require.Error(t, err)
	})

}
