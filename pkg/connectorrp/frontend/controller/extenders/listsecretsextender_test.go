// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
)

func TestListSecrets_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *statusmanager.MockStatusManager, *deployment.MockDeploymentProcessor) {
		mctrl := gomock.NewController(t)
		mds := store.NewMockStorageClient(mctrl)
		msm := statusmanager.NewMockStatusManager(mctrl)
		mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, mds, msm, mDeploymentProcessor
	}
	ctx := context.Background()

	_, extenderDataModel, _ := getTestModels20220315privatepreview()
	expectedSecrets := map[string]interface{}{
		"accountSid": "sid",
		"authToken:": "token",
	}
	t.Run("listSecrets non-existing resource", func(t *testing.T) {
		teardownTest, mds, msm, mDeploymentProcessor := setupTest(t)
		defer teardownTest(t)
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mds.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		opts := ctrl.Options{
			StorageClient:  mds,
			AsyncOperation: msm,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return mDeploymentProcessor
			},
		}

		ctl, err := NewListSecretsExtender(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
	})

	t.Run("listSecrets existing resource", func(t *testing.T) {
		teardownTest, mds, msm, mDeploymentProcessor := setupTest(t)
		defer teardownTest(t)
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mds.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     extenderDataModel,
				}, nil
			})

		mDeploymentProcessor.EXPECT().FetchSecrets(gomock.Any(), gomock.Any()).Times(1).Return(expectedSecrets, nil)

		opts := ctrl.Options{
			StorageClient:  mds,
			AsyncOperation: msm,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return mDeploymentProcessor
			},
		}

		ctl, err := NewListSecretsExtender(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &map[string]interface{}{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		// TODO update to expect secrets values after controller is integrated with backend.
		// require.Equal(t, expectedOutput.Properties.Secrets, actualOutput)
		require.Equal(t, expectedSecrets["accountSid"], (*actualOutput)["accountSid"])
		require.Equal(t, expectedSecrets["authToken"], (*actualOutput)["authToken"])
	})

	t.Run("listSecrets error retrieving resource", func(t *testing.T) {
		teardownTest, mds, msm, mDeploymentProcessor := setupTest(t)
		defer teardownTest(t)
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mds.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, errors.New("failed to get the resource from data store")
			})

		opts := ctrl.Options{
			StorageClient:  mds,
			AsyncOperation: msm,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return mDeploymentProcessor
			},
		}

		ctl, err := NewListSecretsExtender(opts)

		require.NoError(t, err)
		_, err = ctl.Run(ctx, req)
		require.Error(t, err)
	})

}
