// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"
)

func TestGetOperationStatusRun(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	osDataModel, osExpectedOutput := getOperationStatusTestModels20220315privatepreview()

	t.Run("get non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		ctl, err := NewGetOperationStatus(mStorageClient, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, http.StatusNotFound, w.Result().StatusCode)
	})

	t.Run("get existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id},
					Data:     osDataModel,
				}, nil
			})

		ctl, err := NewGetOperationStatus(mStorageClient, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, http.StatusOK, w.Result().StatusCode)

		actualOutput := &datamodel.AsyncOperationStatus{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)

		require.Equal(t, osExpectedOutput, actualOutput)
	})
}
