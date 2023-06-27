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

package extenders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rpctest"
	"github.com/project-radius/radius/pkg/ucp/store"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestListSecrets_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *statusmanager.MockStatusManager) {
		mctrl := gomock.NewController(t)
		mds := store.NewMockStorageClient(mctrl)
		msm := statusmanager.NewMockStatusManager(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, mds, msm
	}
	ctx := context.Background()

	_, extenderDataModel, _ := getTestModels20220315privatepreview()
	expectedSecrets := map[string]any{
		"accountSid": "sid",
		"authToken:": "token",
	}

	t.Run("listSecrets non-existing resource", func(t *testing.T) {
		teardownTest, mds, msm := setupTest(t)
		defer teardownTest(t)
		w := httptest.NewRecorder()
		req, _ := rpctest.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := rpctest.ARMTestContextFromRequest(req)

		mds.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		opts := ctrl.Options{
			StorageClient: mds,
			StatusManager: msm,
		}

		ctl, err := NewListSecretsExtender(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
	})

	t.Run("listSecrets existing resource", func(t *testing.T) {
		teardownTest, mds, msm := setupTest(t)
		defer teardownTest(t)
		w := httptest.NewRecorder()
		req, _ := rpctest.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := rpctest.ARMTestContextFromRequest(req)

		mds.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     extenderDataModel,
				}, nil
			})

		opts := ctrl.Options{
			StorageClient: mds,
			StatusManager: msm,
		}

		ctl, err := NewListSecretsExtender(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := map[string]any{}
		_ = json.Unmarshal(w.Body.Bytes(), &actualOutput)

		require.Equal(t, expectedSecrets["accountSid"], actualOutput["accountSid"])
		require.Equal(t, expectedSecrets["authToken"], actualOutput["authToken"])
	})

	t.Run("listSecrets error retrieving resource", func(t *testing.T) {
		teardownTest, mds, msm := setupTest(t)
		defer teardownTest(t)
		req, _ := rpctest.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := rpctest.ARMTestContextFromRequest(req)
		w := httptest.NewRecorder()

		mds.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, errors.New("failed to get the resource from data store")
			})

		opts := ctrl.Options{
			StorageClient: mds,
			StatusManager: msm,
		}

		ctl, err := NewListSecretsExtender(opts)

		require.NoError(t, err)
		_, err = ctl.Run(ctx, w, req)
		require.Error(t, err)
	})

}
