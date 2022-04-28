// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
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

	t.Parallel()

	t.Run("delete non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodDelete, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		ctl, err := NewDeleteEnvironment(mStorageClient, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 204, w.Result().StatusCode)
	})

	existingResourceDeletionCases := []struct {
		caseNumber         int
		ifMatchEtag        string
		resourceEtag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{1, "", "random-etag", 200, false},
		{2, "", "", 204, true},
		{3, "matching-etag", "matching-etag", 200, false},
		{4, "not-matching-etag", "another-etag", 412, true},
		{5, "*", "", 204, true},
		{6, "*", "random-etag", 200, false},
	}

	for _, tt := range existingResourceDeletionCases {
		t.Run(fmt.Sprint(tt.caseNumber), func(t *testing.T) {
			w := httptest.NewRecorder()

			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodDelete, testHeaderfile, nil)
			req.Header.Set("If-Match", tt.ifMatchEtag)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			_, envDataModel, _ := getTestModels20220315privatepreview()

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceEtag},
						Data:     envDataModel,
					}, nil
				})

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Delete(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.DeleteOptions) error {
						return nil
					})
			}

			ctl, err := NewDeleteEnvironment(mStorageClient, nil)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)
		})
	}
}
