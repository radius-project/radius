// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

func TestGetContainerRun_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient) {
		mctrl := gomock.NewController(t)
		mStorageClient := store.NewMockStorageClient(mctrl)

		return func(tb testing.TB) {
			mctrl.Finish()
		}, mStorageClient
	}

	_, contDataModel, expectedOutput := getTestModels20220315privatepreview()

	t.Run("get non-existing resource", func(t *testing.T) {
		teardownTest, msc := setupTest(t)
		defer teardownTest(t)

		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		msc.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(nil, &store.ErrNotFound{}).
			Times(1)

		opts := ctrl.Options{
			StorageClient: msc,
		}

		ctl, err := NewGetContainer(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
	})

	t.Run("get existing resource", func(t *testing.T) {
		teardownTest, msc := setupTest(t)
		defer teardownTest(t)

		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		msc.EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(&store.Object{Metadata: store.Metadata{ID: contDataModel.ID}, Data: contDataModel}, nil).
			Times(1)

		opts := ctrl.Options{
			StorageClient: msc,
		}

		ctl, err := NewGetContainer(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.ContainerResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, expectedOutput, actualOutput)
	})
}
