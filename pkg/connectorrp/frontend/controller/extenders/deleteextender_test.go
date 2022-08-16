// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestDeleteExtender_20220315PrivatePreview(t *testing.T) {
	setupTest := func(tb testing.TB) (func(tb testing.TB), *store.MockStorageClient, *statusmanager.MockStatusManager, *deployment.MockDeploymentProcessor) {
		mctrl := gomock.NewController(t)
		mds := store.NewMockStorageClient(mctrl)
		msm := statusmanager.NewMockStatusManager(mctrl)
		mDeploymentProcessor := deployment.NewMockDeploymentProcessor(mctrl)
		return func(tb testing.TB) {
			mctrl.Finish()
		}, mds, msm, mDeploymentProcessor
	}

	t.Parallel()

	t.Run("delete non-existing resource", func(t *testing.T) {
		teardownTest, mds, msm, mDeploymentProcessor := setupTest(t)
		defer teardownTest(t)
		w := httptest.NewRecorder()
		req, _ := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderfile, nil)
		ctx := radiustesting.ARMTestContextFromRequest(req)

		mds.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		opts := ctrl.Options{
			StorageClient: mds,
			StatusManager: msm,
			GetDeploymentProcessor: func() deployment.DeploymentProcessor {
				return mDeploymentProcessor
			},
		}

		ctl, err := NewDeleteExtender(opts)

		require.NoError(t, err)
		resp, err := ctl.Run(ctx, req)
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
			teardownTest, mds, msm, mDeploymentProcessor := setupTest(t)
			defer teardownTest(t)
			w := httptest.NewRecorder()

			req, _ := radiustesting.GetARMTestHTTPRequest(context.Background(), http.MethodDelete, testHeaderfile, nil)
			req.Header.Set("If-Match", testcase.ifMatchETag)

			ctx := radiustesting.ARMTestContextFromRequest(req)
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
				mDeploymentProcessor.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
				mds.
					EXPECT().
					Delete(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, id string, _ ...store.DeleteOptions) error {
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mds,
				StatusManager: msm,
				GetDeploymentProcessor: func() deployment.DeploymentProcessor {
					return mDeploymentProcessor
				},
			}

			ctl, err := NewDeleteExtender(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, req)
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
				armerr := armerrors.ErrorResponse{}
				err = json.Unmarshal(payload, &armerr)
				require.NoError(t, err)
				require.Equal(t, armerrors.PreconditionFailed, armerr.Error.Code)
				require.NotEmpty(t, armerr.Error.Target)
			}
		})
	}
}
