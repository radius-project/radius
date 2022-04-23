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
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"
)

func TestDeleteEnvironmentRun_20220315privatepreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	const headerfile = "requestheaders20220315privatepreview.json"
	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	t.Run("delete non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := GetTestHTTPRequest(ctx, http.MethodDelete, headerfile, nil)
		ctx := GetTestRequestContext(req)

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
		resp.Apply(ctx, w, req)
		require.Equal(t, 204, w.Result().StatusCode)
	})

	t.Run("delete existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := GetTestHTTPRequest(ctx, http.MethodDelete, headerfile, nil)
		ctx := GetTestRequestContext(req)

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
		resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)
	})
}
