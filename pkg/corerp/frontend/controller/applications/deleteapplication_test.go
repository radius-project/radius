// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package applications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestDeleteApplicationRun_20220315PrivatePreview(t *testing.T) {
	tCtx := testutil.NewTestContext(t)

	t.Parallel()

	t.Run("delete non-existing resource", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(tCtx.Ctx, http.MethodDelete, testHeaderfile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)

		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		opts := ctrl.Options{
			StorageClient: tCtx.MockSC,
		}

		ctl, err := NewDeleteApplication(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		err = resp.Apply(ctx, w, req)
		require.NoError(t, err)

		result := w.Result()
		require.Equal(t, http.StatusNoContent, result.StatusCode)

		body := result.Body
		defer body.Close()
		payload, err := io.ReadAll(body)
		require.NoError(t, err)
		require.Empty(t, payload, "response body should be empty")
	})

	existingResourceDeletionCases := []struct {
		desc               string
		ifMatchETag        string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"delete-existing-resource-no-if-match", "", "random-etag", 200, false},
		{"delete-not-existing-resource-no-if-match", "", "", 204, true},
		{"delete-existing-resource-matching-if-match", "matching-etag", "matching-etag", 200, false},
		{"delete-existing-resource-not-matching-if-match", "not-matching-etag", "another-etag", 412, true},
		{"delete-not-existing-resource-*-if-match", "*", "", 204, true},
		{"delete-existing-resource-*-if-match", "*", "random-etag", 200, false},
	}

	for _, tt := range existingResourceDeletionCases {
		t.Run(tt.desc, func(t *testing.T) {
			w := httptest.NewRecorder()

			req, _ := testutil.GetARMTestHTTPRequest(tCtx.Ctx, http.MethodDelete, testHeaderfile, nil)
			req.Header.Set("If-Match", tt.ifMatchETag)

			ctx := testutil.ARMTestContextFromRequest(req)
			_, appDataModel, _ := getTestModels20220315privatepreview()

			tCtx.MockSC.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					if tt.expectedStatusCode == 204 {
						return nil, &store.ErrNotFound{}
					}

					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceETag},
						Data:     appDataModel,
					}, nil
				})

			if !tt.shouldFail {
				tCtx.MockSC.
					EXPECT().
					Delete(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.DeleteOptions) error {
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: tCtx.MockSC,
				KubeClient:    testutil.NewFakeKubeClient(nil),
			}

			ctl, err := NewDeleteApplication(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			err = resp.Apply(ctx, w, req)
			require.NoError(t, err)

			result := w.Result()
			require.Equal(t, tt.expectedStatusCode, result.StatusCode)

			body := result.Body
			defer body.Close()
			payload, err := io.ReadAll(body)
			require.NoError(t, err)

			if result.StatusCode == 200 || result.StatusCode == 204 {
				require.Empty(t, payload, "response body should be empty")
			} else {
				armerr := v1.ErrorResponse{}
				err = json.Unmarshal(payload, &armerr)
				require.NoError(t, err)
				require.Equal(t, v1.CodePreconditionFailed, armerr.Error.Code)
				require.NotEmpty(t, armerr.Error.Target)
			}
		})
	}
}
