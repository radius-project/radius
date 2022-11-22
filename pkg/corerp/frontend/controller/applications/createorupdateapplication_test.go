// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package applications

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func fakeStoreObject(dm conv.DataModelInterface) *store.Object {
	b, err := json.Marshal(dm)
	if err != nil {
		return nil
	}
	var r any
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil
	}
	return &store.Object{Data: r}
}

func TestCreateOrUpdateApplicationRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	ctx := context.Background()

	envdm := &datamodel.Environment{
		Properties: datamodel.EnvironmentProperties{
			Compute: datamodel.EnvironmentCompute{
				Kind: datamodel.KubernetesComputeKind,
				KubernetesCompute: datamodel.KubernetesComputeProperties{
					Namespace: "default",
				},
			},
		},
	}

	createNewResourceCases := []struct {
		desc               string
		headerKey          string
		headerValue        string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"create-new-resource-no-if-match", "If-Match", "", "", 200, false},
		{"create-new-resource-*-if-match", "If-Match", "*", "", 412, true},
		{"create-new-resource-etag-if-match", "If-Match", "randome-etag", "", 412, true},
		{"create-new-resource-*-if-none-match", "If-None-Match", "*", "", 200, false},
	}

	for _, tt := range createNewResourceCases {
		t.Run(tt.desc, func(t *testing.T) {
			appInput, appDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				}).Times(1)

			if !tt.shouldFail {
				// Mocks for FindNamespaceByEnvID
				mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mStorageClient), nil).Times(1)
				mStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(fakeStoreObject(envdm), nil)

				// Environmment and application namespace queries
				mStorageClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					}).Times(2)
			}

			expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
			expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
			expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						obj.ETag = "new-resource-etag"
						obj.Data = appDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
				DataProvider:  mockSP,
			}

			ctl, err := NewCreateOrUpdateApplication(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20220315privatepreview.ApplicationResource{}
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
		inputFile          string
		resourceETag       string
		expectedStatusCode int
		shouldFail         bool
	}{
		{"update-resource-no-if-match", "If-Match", "", "", "resource-etag", 200, false},
		{"update-resource-with-diff-env", "If-Match", "", "application20220315privatepreview_input_diff_env.json", "resource-etag", 400, true},
		{"update-resource-*-if-match", "If-Match", "*", "", "resource-etag", 200, false},
		{"update-resource-matching-if-match", "If-Match", "matching-etag", "", "matching-etag", 200, false},
		{"update-resource-not-matching-if-match", "If-Match", "not-matching-etag", "", "another-etag", 412, true},
		{"update-resource-*-if-none-match", "If-None-Match", "*", "", "another-etag", 412, true},
	}

	for _, tt := range updateExistingResourceCases {
		t.Run(tt.desc, func(t *testing.T) {
			appInput, appDataModel, expectedOutput := getTestModels20220315privatepreview()
			if tt.inputFile != "" {
				appInput = &v20220315privatepreview.ApplicationResource{}
				_ = json.Unmarshal(radiustesting.ReadFixture(tt.inputFile), appInput)
			}
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceETag},
						Data:     appDataModel,
					}, nil
				})

			if !tt.shouldFail {
				// Mocks for FindNamespaceByEnvID
				mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mStorageClient), nil).Times(1)
				mStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(fakeStoreObject(envdm), nil)

				// Environmment and application namespace queries
				mStorageClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					}).Times(2)

				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						obj.ETag = "updated-resource-etag"
						obj.Data = appDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
				DataProvider:  mockSP,
			}

			ctl, err := NewCreateOrUpdateApplication(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20220315privatepreview.ApplicationResource{}
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
		{"patch-non-existing-resource-no-if-match", "If-Match", "", "", 404, true},
		{"patch-non-existing-resource-*-if-match", "If-Match", "*", "", 404, true},
		{"patch-non-existing-resource-random-if-match", "If-Match", "randome-etag", "", 404, true},
	}

	for _, tt := range patchNonExistingResourceCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			appInput, _, _ := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodPatch, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				})

			if !tt.shouldFail {
				// Mocks for FindNamespaceByEnvID
				mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mStorageClient), nil).Times(1)
				mStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(fakeStoreObject(envdm), nil)

				// Environmment and application namespace queries
				mStorageClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					}).Times(2)
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
				DataProvider:  mockSP,
			}

			ctl, err := NewCreateOrUpdateApplication(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
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
		{"patch-existing-resource-no-if-match", "If-Match", "", "resource-etag", 200, false},
		{"patch-existing-resource-*-if-match", "If-Match", "*", "resource-etag", 200, false},
		{"patch-existing-resource-matching-if-match", "If-Match", "matching-etag", "matching-etag", 200, false},
		{"patch-existing-resource-not-matching-if-match", "If-Match", "not-matching-etag", "another-etag", 412, true},
	}

	for _, tt := range patchExistingResourceCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			appInput, appDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodPatch, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceEtag},
						Data:     appDataModel,
					}, nil
				})

			if !tt.shouldFail {
				// Mocks for FindNamespaceByEnvID
				mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mStorageClient), nil).Times(1)
				mStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(fakeStoreObject(envdm), nil)

				// Environmment and application namespace queries
				mStorageClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					}).Times(2)

				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						cfg := store.NewSaveConfig(opts...)
						obj.ETag = cfg.ETag
						obj.Data = appDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
				DataProvider:  mockSP,
			}

			ctl, err := NewCreateOrUpdateApplication(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20220315privatepreview.ApplicationResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)
			}
		})
	}

	existingResourceNamespaceCases := []struct {
		desc                 string
		headerKey            string
		headerValue          string
		resourceEtag         string
		existingResourceName string
		expectedStatusCode   int
		shouldFail           bool
	}{
		{"create-existing-namespace-match", "If-Match", "", "resource-etag", "app1", 409, true},
		{"create-existing-namespace-match-same-resource", "If-Match", "", "resource-etag", "app0", 200, false},
	}

	for _, tt := range existingResourceNamespaceCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			appInput, appDataModel, _ := getTestModels20220315privatepreview()
			_, conflictDataModel, _ := getTestModels20220315privatepreview()

			conflictDataModel.Name = "existing"
			conflictDataModel.ID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/" + tt.existingResourceName
			w := httptest.NewRecorder()
			req, _ := radiustesting.GetARMTestHTTPRequest(ctx, http.MethodPatch, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := radiustesting.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceEtag},
						Data:     appDataModel,
					}, nil
				})

			paginationToken := "nextLink"

			items := []store.Object{
				{
					Metadata: store.Metadata{
						ID: uuid.New().String(),
					},
					Data: conflictDataModel,
				},
			}

			// Mocks for FindNamespaceByEnvID
			mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mStorageClient), nil).Times(1)
			mStorageClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(fakeStoreObject(envdm), nil)

			// Environment namespace query
			mStorageClient.
				EXPECT().
				Query(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
					return &store.ObjectQueryResult{
						Items: []store.Object{},
					}, nil
				})

			// Application namespace query
			mStorageClient.
				EXPECT().
				Query(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
					return &store.ObjectQueryResult{
						Items:           items,
						PaginationToken: paginationToken,
					}, nil
				})

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						cfg := store.NewSaveConfig(opts...)
						obj.ETag = cfg.ETag
						obj.Data = appDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
				DataProvider:  mockSP,
			}

			ctl, err := NewCreateOrUpdateApplication(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)
		})
	}

}
