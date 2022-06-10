// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateHttpRouteRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	createNewResourceCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"create-new-resource-no-if-match", "If-Match", "", "", http.StatusOK, false},
		{"create-new-resource-*-if-match", "If-Match", "*", "", http.StatusPreconditionFailed, true},
		{"create-new-resource-etag-if-match", "If-Match", "random-etag", "", http.StatusPreconditionFailed, true},
		{"create-new-resource-*-if-none-match", "If-None-Match", "*", "", http.StatusOK, false},
	}

	for _, tt := range createNewResourceCases {
		t.Run(tt.desc, func(t *testing.T) {
			hrtInput, hrtDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, hrtInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				})

			expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
			expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
			expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						obj.ETag = "new-resource-etag"
						obj.Data = hrtDataModel
						return nil
					})
			}

			ctl, err := NewCreateOrUpdateHTTPRoute(mStorageClient, nil)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20220315privatepreview.HTTPRouteResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)

				require.Equal(t, "new-resource-etag", w.Header().Get("ETag"))
			}
		})
	}

	updateExistingResourceCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"update-resource-no-if-match", "If-Match", "", "resource-etag", http.StatusOK, false},
		{"update-resource-*-if-match", "If-Match", "*", "resource-etag", http.StatusOK, false},
		{"update-resource-matching-if-match", "If-Match", "matching-etag", "matching-etag", http.StatusOK, false},
		{"update-resource-not-matching-if-match", "If-Match", "not-matching-etag", "another-etag", http.StatusPreconditionFailed, true},
		{"update-resource-*-if-none-match", "If-None-Match", "*", "another-etag", http.StatusPreconditionFailed, true},
	}

	for _, tt := range updateExistingResourceCases {
		t.Run(tt.desc, func(t *testing.T) {
			hrtInput, hrtDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, hrtInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceETag},
						Data:     hrtDataModel,
					}, nil
				})

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						obj.ETag = "updated-resource-etag"
						obj.Data = hrtDataModel
						return nil
					})
			}

			ctl, err := NewCreateOrUpdateHTTPRoute(mStorageClient, nil)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, req)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20220315privatepreview.HTTPRouteResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)

				require.Equal(t, "updated-resource-etag", w.Header().Get("ETag"))
			}
		})
	}

	patchNonExistingResourceCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceEtag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"patch-non-existing-resource-no-if-match", "If-Match", "", "", http.StatusNotFound, true},
		{"patch-non-existing-resource-*-if-match", "If-Match", "*", "", http.StatusNotFound, true},
		{"patch-non-existing-resource-random-if-match", "If-Match", "randome-etag", "", http.StatusNotFound, true},
	}

	for _, tt := range patchNonExistingResourceCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			hrtInput, _, _ := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodPatch, testHeaderfile, hrtInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				})

			ctl, err := NewCreateOrUpdateHTTPRoute(mStorageClient, nil)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)
		})
	}

	patchExistingResourceCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceEtag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"patch-existing-resource-no-if-match", "If-Match", "", "resource-etag", http.StatusOK, false},
		{"patch-existing-resource-*-if-match", "If-Match", "*", "resource-etag", http.StatusOK, false},
		{"patch-existing-resource-matching-if-match", "If-Match", "matching-etag", "matching-etag", http.StatusOK, false},
		{"patch-existing-resource-not-matching-if-match", "If-Match", "not-matching-etag", "another-etag", http.StatusPreconditionFailed, true},
	}

	for _, tt := range patchExistingResourceCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			hrtInput, hrtDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodPatch, testHeaderfile, hrtInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceEtag},
						Data:     hrtDataModel,
					}, nil
				})

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						cfg := store.NewSaveConfig(opts...)
						obj.ETag = cfg.ETag
						obj.Data = hrtDataModel
						return nil
					})
			}

			ctl, err := NewCreateOrUpdateHTTPRoute(mStorageClient, nil)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, req)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20220315privatepreview.HTTPRouteResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)
			}
		})
	}
}
