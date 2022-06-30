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
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
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

	t.Run("listSecrets non-existing resource", func(t *testing.T) {
		teardownTest, mds, msm := setupTest(t)
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

		ctl, err := NewListSecretsExtender(mds, msm, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
	})

	t.Run("listSecrets existing resource", func(t *testing.T) {
		teardownTest, mds, msm := setupTest(t)
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

		ctl, err := NewListSecretsExtender(mds, msm, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.ExtenderResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		// TODO update to expect secrets values after controller is integrated with backend.
		// require.Equal(t, expectedOutput.Properties.Secrets, actualOutput)
		require.Equal(t, &v20220315privatepreview.ExtenderResource{}, actualOutput)
	})

	t.Run("listSecrets error retrieving resource", func(t *testing.T) {
		teardownTest, mds, msm := setupTest(t)
		defer teardownTest(t)
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mds.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, errors.New("failed to get the resource from data store")
			})

		ctl, err := NewListSecretsExtender(mds, msm, nil)

		require.NoError(t, err)
		_, err = ctl.Run(ctx, req)
		require.Error(t, err)
	})

}
