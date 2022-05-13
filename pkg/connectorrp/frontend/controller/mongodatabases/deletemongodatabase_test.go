// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/store"
	"github.com/stretchr/testify/require"
)

func TestDeleteMongoDatabase_20220315PrivatePreview(t *testing.T) {
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

		ctl, err := NewDeleteMongoDatabase(mStorageClient, nil)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
		require.NoError(t, err)
		err = resp.Apply(ctx, w, req)
		require.NoError(t, err)

		result := w.Result()
		require.Equal(t, http.StatusNoContent, result.StatusCode)

		body := result.Body
		defer body.Close()
		payload, err := ioutil.ReadAll(body)
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
			w := httptest.NewRecorder()

			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodDelete, testHeaderfile, nil)
			req.Header.Set("If-Match", testcase.ifMatchETag)

			ctx := radiustesting.ARMTestContextFromRequest(req)
			_, mongoDataModel, _ := getTestModels20220315privatepreview()

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: testcase.resourceETag},
						Data:     mongoDataModel,
					}, nil
				})

			if !testcase.shouldFail {
				mStorageClient.
					EXPECT().
					Delete(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.DeleteOptions) error {
						return nil
					})
			}

			ctl, err := NewDeleteMongoDatabase(mStorageClient, nil)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, req)
			require.NoError(t, err)
			err = resp.Apply(ctx, w, req)
			require.NoError(t, err)

			result := w.Result()
			require.Equal(t, testcase.expectedStatusCode, result.StatusCode)

			body := result.Body
			defer body.Close()
			payload, err := ioutil.ReadAll(body)
			require.NoError(t, err)

			if result.StatusCode == http.StatusOK || result.StatusCode == http.StatusNoContent {
				// We return either 200 or 204 without a response body for success.
				require.Empty(t, payload, "response body should be empty")
			} else {
				armerr := armerrors.ErrorResponse{}
				err = json.Unmarshal(payload, &armerr)
				require.NoError(t, err)
				require.Equal(t, armerrors.PreconditionFailed, armerr.Error.Code)
				require.NotEmpty(t, armerr.Error.Target)
			}
		})
	}
}
