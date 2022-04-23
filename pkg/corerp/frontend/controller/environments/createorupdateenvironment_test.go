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
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateEnvironmentRun_20220315privatepreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	const headerfile = "requestheaders20220315privatepreview.json"
	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	t.Run("create new resource", func(t *testing.T) {
		envInput, envDataModel, expectedOutput := GetTestModels20220315privatepreview()

		w := httptest.NewRecorder()
		req, _ := GetTestHTTPRequest(ctx, http.MethodGet, headerfile, envInput)
		ctx := GetTestRequestContext(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

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

		ctl, err := NewCreateOrUpdateEnvironment(mStorageClient, nil)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)
		actualOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})

	t.Run("update existing resource", func(t *testing.T) {
		envInput, envDataModel, expectedOutput := GetTestModels20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := GetTestHTTPRequest(ctx, http.MethodGet, headerfile, envInput)
		ctx := GetTestRequestContext(req)

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

		ctl, err := NewCreateOrUpdateEnvironment(mStorageClient, nil)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)
		actualOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})
}
