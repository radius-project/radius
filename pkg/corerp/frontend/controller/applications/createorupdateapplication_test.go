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

package applications

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/k8sutil"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	testAppID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/app0"
	testEnvID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
)

func TestCreateOrUpdateApplicationRun_CreateNew_20220315PrivatePreview(t *testing.T) {
	tCtx := testutil.NewTestContext(t)

	ctrlTests := []struct {
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

	for _, tt := range ctrlTests {
		t.Run(tt.desc, func(t *testing.T) {
			appInput, appDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(tCtx.Ctx, http.MethodGet, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			tCtx.MockSC.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				}).Times(1)

			if !tt.shouldFail {
				// Environmment and application namespace queries
				tCtx.MockSC.
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
				tCtx.MockSC.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						obj.ETag = "new-resource-etag"
						obj.Data = appDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: tCtx.MockSC,
				DataProvider:  tCtx.MockSP,
				KubeClient:    k8sutil.NewFakeKubeClient(nil),
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
}

func TestCreateOrUpdateApplicationRun_Update_20220315PrivatePreview(t *testing.T) {
	tCtx := testutil.NewTestContext(t)

	ctrlTests := []struct {
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

	for _, tt := range ctrlTests {
		t.Run(tt.desc, func(t *testing.T) {
			appInput, appDataModel, expectedOutput := getTestModels20220315privatepreview()
			if tt.inputFile != "" {
				appInput = &v20220315privatepreview.ApplicationResource{}
				_ = json.Unmarshal(testutil.ReadFixture(tt.inputFile), appInput)
			}
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(tCtx.Ctx, http.MethodGet, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			tCtx.MockSC.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceETag},
						Data:     appDataModel,
					}, nil
				})

			if !tt.shouldFail {
				// Environmment and application namespace queries
				tCtx.MockSC.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					}).Times(2)

				tCtx.MockSC.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						obj.ETag = "updated-resource-etag"
						obj.Data = appDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: tCtx.MockSC,
				DataProvider:  tCtx.MockSP,
				KubeClient:    k8sutil.NewFakeKubeClient(nil),
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
}

func TestCreateOrUpdateApplicationRun_PatchNonExisting_20220315PrivatePreview(t *testing.T) {
	tCtx := testutil.NewTestContext(t)

	ctrlTests := []struct {
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

	for _, tt := range ctrlTests {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			appInput, _, _ := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(tCtx.Ctx, http.MethodPatch, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			tCtx.MockSC.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				})

			if !tt.shouldFail {
				// Environmment and application namespace queries
				tCtx.MockSC.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					}).Times(2)
			}

			opts := ctrl.Options{
				StorageClient: tCtx.MockSC,
				DataProvider:  tCtx.MockSP,
				KubeClient:    k8sutil.NewFakeKubeClient(nil),
			}

			ctl, err := NewCreateOrUpdateApplication(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)
		})
	}
}

func TestCreateOrUpdateApplicationRun_PatchExisting_20220315PrivatePreview(t *testing.T) {
	tCtx := testutil.NewTestContext(t)

	ctrlTests := []struct {
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

	for _, tt := range ctrlTests {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			appInput, appDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(tCtx.Ctx, http.MethodPatch, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			tCtx.MockSC.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceEtag},
						Data:     appDataModel,
					}, nil
				})

			if !tt.shouldFail {
				// Environmment and application namespace queries
				tCtx.MockSC.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					}).Times(2)

				tCtx.MockSC.
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
				StorageClient: tCtx.MockSC,
				DataProvider:  tCtx.MockSP,
				KubeClient:    k8sutil.NewFakeKubeClient(nil),
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
}

func TestCreateOrUpdateApplicationRun_CreateExisting_20220315PrivatePreview(t *testing.T) {
	tCtx := testutil.NewTestContext(t)

	ctrlTests := []struct {
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

	for _, tt := range ctrlTests {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			appInput, appDataModel, _ := getTestModels20220315privatepreview()
			_, conflictDataModel, _ := getTestModels20220315privatepreview()

			conflictDataModel.Name = "existing"
			conflictDataModel.ID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/" + tt.existingResourceName
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(tCtx.Ctx, http.MethodPatch, testHeaderfile, appInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			tCtx.MockSC.
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

			// Environment namespace query
			tCtx.MockSC.
				EXPECT().
				Query(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
					return &store.ObjectQueryResult{
						Items: []store.Object{},
					}, nil
				})

			// Application namespace query
			tCtx.MockSC.
				EXPECT().
				Query(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
					return &store.ObjectQueryResult{
						Items:           items,
						PaginationToken: paginationToken,
					}, nil
				})

			if !tt.shouldFail {
				tCtx.MockSC.
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
				StorageClient: tCtx.MockSC,
				DataProvider:  tCtx.MockSP,
				KubeClient:    k8sutil.NewFakeKubeClient(nil),
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

func TestPopulateKubernetesNamespace_valid_namespace(t *testing.T) {
	tCtx := testutil.NewTestContext(t)

	opts := ctrl.Options{
		StorageClient: tCtx.MockSC,
		DataProvider:  tCtx.MockSP,
		KubeClient:    k8sutil.NewFakeKubeClient(nil),
	}

	ctl, err := NewCreateOrUpdateApplication(opts)
	require.NoError(t, err)
	appCtrl := ctl.(*CreateOrUpdateApplication)

	t.Run("override namespace", func(t *testing.T) {
		old := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
					Status: rpv1.ResourceStatus{
						Compute: &rpv1.EnvironmentCompute{
							Kind: rpv1.KubernetesComputeKind,
							KubernetesCompute: rpv1.KubernetesComputeProperties{
								Namespace: "app-ns",
							},
						},
					},
				},
			},
		}

		tCtx.MockSC.
			EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			}).Times(2)

		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "app-ns"},
					},
				},
			},
		}

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := appCtrl.populateKubernetesNamespace(ctx, newResource, old)
		require.NoError(t, err)
		require.Nil(t, resp)

		require.Equal(t, "app-ns", newResource.Properties.Status.Compute.KubernetesCompute.Namespace)
	})

	t.Run("generate namespace with environment", func(t *testing.T) {
		tCtx.MockSC.
			EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			}).Times(2)

		tCtx.MockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(tCtx.MockSC, nil).Times(1)

		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.KubernetesComputeKind,
					KubernetesCompute: rpv1.KubernetesComputeProperties{
						Namespace: "default",
					},
				},
			},
		}

		tCtx.MockSC.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(testutil.FakeStoreObject(envdm), nil)

		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
			},
		}

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := appCtrl.populateKubernetesNamespace(ctx, newResource, nil)
		require.NoError(t, err)
		require.Nil(t, resp)

		require.Equal(t, "default-app0", newResource.Properties.Status.Compute.KubernetesCompute.Namespace)
	})
}

func TestPopulateKubernetesNamespace_invalid_property(t *testing.T) {
	tCtx := testutil.NewTestContext(t)

	opts := ctrl.Options{
		StorageClient: tCtx.MockSC,
		DataProvider:  tCtx.MockSP,
		KubeClient:    k8sutil.NewFakeKubeClient(nil),
	}

	ctl, err := NewCreateOrUpdateApplication(opts)
	require.NoError(t, err)
	appCtrl := ctl.(*CreateOrUpdateApplication)

	t.Run("invalid namespace", func(t *testing.T) {
		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			}).Times(2)

		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: strings.Repeat("invalid-name", 6)},
					},
				},
			},
		}

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := appCtrl.populateKubernetesNamespace(ctx, newResource, nil)
		require.NoError(t, err)
		res := resp.(*rest.BadRequestResponse)
		require.Equal(t, res.Body.Error.Message, "'invalid-nameinvalid-nameinvalid-nameinvalid-nameinvalid-nameinvalid-name' is the invalid namespace. This must be at most 63 alphanumeric characters or '-'. Please specify a valid namespace using 'kubernetesNamespace' extension in '$.properties.extensions[*]'.")
	})

	t.Run("conflicted namespace in environment resource", func(t *testing.T) {
		envdm := &datamodel.Environment{
			Properties: datamodel.EnvironmentProperties{
				Compute: rpv1.EnvironmentCompute{
					Kind: rpv1.KubernetesComputeKind,
					KubernetesCompute: rpv1.KubernetesComputeProperties{
						Namespace: "testns",
					},
				},
			},
		}

		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{*testutil.FakeStoreObject(envdm)},
				}, nil
			}).Times(1)

		newResource := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "testns"},
					},
				},
			},
		}

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := appCtrl.populateKubernetesNamespace(ctx, newResource, nil)
		require.NoError(t, err)
		res := resp.(*rest.ConflictResponse)
		require.Equal(t, res.Body.Error.Message, "Environment env0 with the same namespace (testns) already exists")
	})

	t.Run("conflicted namespace in the different application resource", func(t *testing.T) {
		newResource := &datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: testAppID,
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "testns"},
					},
				},
			},
		}

		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			Return(&store.ObjectQueryResult{}, nil).Times(1)
		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			Return(&store.ObjectQueryResult{Items: []store.Object{*testutil.FakeStoreObject(newResource)}}, nil).Times(1)

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := appCtrl.populateKubernetesNamespace(ctx, newResource, nil)
		require.NoError(t, err)
		res := resp.(*rest.ConflictResponse)
		require.Equal(t, res.Body.Error.Message, "Application /subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/app0 with the same namespace (testns) already exists")
	})

	t.Run("update application with the different namespace", func(t *testing.T) {
		old := &datamodel.Application{
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
					Status: rpv1.ResourceStatus{
						Compute: &rpv1.EnvironmentCompute{
							Kind: rpv1.KubernetesComputeKind,
							KubernetesCompute: rpv1.KubernetesComputeProperties{
								Namespace: "default-app0",
							},
						},
					},
				},
			},
		}

		newResource := &datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: testAppID,
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: testEnvID,
				},
				Extensions: []datamodel.Extension{
					{
						Kind:                datamodel.KubernetesNamespaceExtension,
						KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "differentname"},
					},
				},
			},
		}

		tCtx.MockSC.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			Return(&store.ObjectQueryResult{}, nil).Times(2)

		id, err := resources.ParseResource(testAppID)
		require.NoError(t, err)
		armctx := &v1.ARMRequestContext{ResourceID: id}
		ctx := v1.WithARMRequestContext(tCtx.Ctx, armctx)

		resp, err := appCtrl.populateKubernetesNamespace(ctx, newResource, old)
		require.NoError(t, err)
		res := resp.(*rest.BadRequestResponse)
		require.Equal(t, res.Body.Error.Message, "Updating an application's Kubernetes namespace from 'default-app0' to 'differentname' requires the application to be deleted and redeployed. Please delete your application and try again.")
	})
}
