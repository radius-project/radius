/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package extenders

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

func TestDeleteExtender_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient) {
		mctrl := gomock.NewController(t)
		mds := store.NewMockStorageClient(mctrl)
		return func(tb testing.TB) {
			mctrl.Finish()
		}, mds
	}

	t.Parallel()

	t.Run("delete non-existing resource", func(t *testing.T) {
		teardownTest, mds := setupTest(t)
		defer teardownTest(t)
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderfile, nil)
		ctx := testutil.ARMTestContextFromRequest(req)

		mds.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		opts := ctrl.Options{
			StorageClient: mds,
		}

		ctl, err := NewDeleteExtender(opts)

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

	existingResourceDeleteTestCases := []struct {
		desc               string
		ifMatchETag        string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"delete-existing-resource-no-if-match", "", "random-etag", http.StatusOK, false},
		{"delete-not-existing-resource-no-if-match", "", "", http.StatusNoContent, true},
		{"delete-existing-resource-matching-if-match", "matching-etag", "matching-etag", http.StatusOK, false},
		{"delete-existing-resource-not-matching-if-match", "not-matching-etag", "another-etag", http.StatusPreconditionFailed, true},
		{"delete-not-existing-resource-*-if-match", "*", "", http.StatusNoContent, true},
		{"delete-existing-resource-*-if-match", "*", "random-etag", http.StatusOK, false},
	}

	for _, testcase := range existingResourceDeleteTestCases {
		t.Run(testcase.desc, func(t *testing.T) {
			teardownTest, mds := setupTest(t)
			defer teardownTest(t)
			w := httptest.NewRecorder()

			req, _ := testutil.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderfile, nil)
			req.Header.Set("If-Match", testcase.ifMatchETag)

			ctx := testutil.ARMTestContextFromRequest(req)
			_, extenderDataModel, _ := getTestModels20220315privatepreview()

			mds.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: testcase.resourceETag},
						Data:     extenderDataModel,
					}, nil
				})

			if !testcase.shouldFail {
				mds.
					EXPECT().
					Delete(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.DeleteOptions) error {
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mds,
			}

			ctl, err := NewDeleteExtender(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			err = resp.Apply(ctx, w, req)
			require.NoError(t, err)

			result := w.Result()
			require.Equal(t, testcase.expectedStatusCode, result.StatusCode)

			body := result.Body
			defer body.Close()
			payload, err := io.ReadAll(body)
			require.NoError(t, err)

			if result.StatusCode == http.StatusOK || result.StatusCode == http.StatusNoContent {
				// We return either 200 or 204 without a response body for success.
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
