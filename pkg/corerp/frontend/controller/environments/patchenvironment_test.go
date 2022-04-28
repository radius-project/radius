// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

func TestPatchEnvironmentRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	t.Run("patch non-existing resource", func(t *testing.T) {
		envInput, _, _ := getTestModels20220315privatepreview()

		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		ctl, err := NewPatchEnvironment(mStorageClient, nil)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
	})

	t.Run("patch existing resource", func(t *testing.T) {
		envInput, envDataModel, expectedOutput := getTestModels20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id, ETag: "fakeEtag"},
					Data:     envDataModel,
				}, nil
			})

		mStorageClient.
			EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) (*store.Object, error) {
				cfg := store.NewSaveConfig(opts...)
				return &store.Object{
					Metadata: store.Metadata{ID: obj.ID, ETag: cfg.ETag},
					Data:     envDataModel,
				}, nil
			})

		ctl, err := NewPatchEnvironment(mStorageClient, nil)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		_ = resp.Apply(ctx, w, req)
		require.NoError(t, err)
		require.Equal(t, 200, w.Result().StatusCode)
		actualOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})
}
