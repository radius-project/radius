// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"
)

func TestDeleteEnvironmentRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	t.Run("delete non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodDelete, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.DeleteOptions) error {
				return &store.ErrNotFound{}
			})

		ctl, err := NewDeleteEnvironment(mStorageClient, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, nil)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 204, w.Result().StatusCode)
	})

	t.Run("delete existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodDelete, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Delete(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.DeleteOptions) error {
				return nil
			})

		ctl, err := NewDeleteEnvironment(mStorageClient, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, nil)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)
	})
}
