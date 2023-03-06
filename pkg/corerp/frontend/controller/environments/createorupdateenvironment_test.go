// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateOrUpdateEnvironmentRun_20220315PrivatePreview(t *testing.T) {
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
		{"create-new-resource-no-if-match", "If-Match", "", "", 200, false},
		{"create-new-resource-*-if-match", "If-Match", "*", "", 412, true},
		{"create-new-resource-etag-if-match", "If-Match", "randome-etag", "", 412, true},
		{"create-new-resource-*-if-none-match", "If-None-Match", "*", "", 200, false},
	}

	for _, tt := range createNewResourceCases {
		t.Run(tt.desc, func(t *testing.T) {
			envInput, envDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				})

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					})
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
						obj.Data = envDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
			}

			ctl, err := NewCreateOrUpdateEnvironment(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20220315privatepreview.EnvironmentResource{}
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
		{"update-resource-no-if-match", "If-Match", "", "resource-etag", 200, false},
		{"update-resource-*-if-match", "If-Match", "*", "resource-etag", 200, false},
		{"update-resource-matching-if-match", "If-Match", "matching-etag", "matching-etag", 200, false},
		{"update-resource-not-matching-if-match", "If-Match", "not-matching-etag", "another-etag", 412, true},
		{"update-resource-*-if-none-match", "If-None-Match", "*", "another-etag", 412, true},
	}

	for _, tt := range updateExistingResourceCases {
		t.Run(tt.desc, func(t *testing.T) {
			envInput, envDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceETag},
						Data:     envDataModel,
					}, nil
				})

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					})
			}

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						obj.ETag = "updated-resource-etag"
						obj.Data = envDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
			}

			ctl, err := NewCreateOrUpdateEnvironment(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)

			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20220315privatepreview.EnvironmentResource{}
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
			envInput, _, _ := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodPatch, testHeaderfile, envInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return nil, &store.ErrNotFound{}
				})

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
			}

			ctl, err := NewCreateOrUpdateEnvironment(opts)
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
			envInput, envDataModel, expectedOutput := getTestModels20220315privatepreview()
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodPatch, testHeaderfile, envInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

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
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
						return &store.ObjectQueryResult{
							Items: []store.Object{},
						}, nil
					})
			}

			if !tt.shouldFail {
				mStorageClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
						cfg := store.NewSaveConfig(opts...)
						obj.ETag = cfg.ETag
						obj.Data = envDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
			}

			ctl, err := NewCreateOrUpdateEnvironment(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20220315privatepreview.EnvironmentResource{}
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
		{"create-existing-namespace-match", "If-Match", "", "resource-etag", "env1", 409, true},
		{"create-existing-namespace-match-same-resource", "If-Match", "", "resource-etag", "env0", 200, false},
	}

	for _, tt := range existingResourceNamespaceCases {
		t.Run(fmt.Sprint(tt.desc), func(t *testing.T) {
			envInput, envDataModel, _ := getTestModels20220315privatepreview()
			_, conflictDataModel, _ := getTestModels20220315privatepreview()

			conflictDataModel.Name = "existing"
			conflictDataModel.ID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/" + tt.existingResourceName
			w := httptest.NewRecorder()
			req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodPatch, testHeaderfile, envInput)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := testutil.ARMTestContextFromRequest(req)

			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: tt.resourceEtag},
						Data:     envDataModel,
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
						obj.Data = envDataModel
						return nil
					})
			}

			opts := ctrl.Options{
				StorageClient: mStorageClient,
			}

			ctl, err := NewCreateOrUpdateEnvironment(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)
		})
	}

}

var mockgetDevRecipes = func(ctx context.Context) (map[string]datamodel.EnvironmentRecipeProperties, error) {

	recipes := map[string]datamodel.EnvironmentRecipeProperties{
		"redis-kubernetes": {
			LinkType:     "Applications.Link/redisCaches",
			TemplatePath: "radius.azurecr.io/recipes/rediscaches/kubernetes:1.0",
		},
		"mongo-azure": {
			LinkType:     "Applications.Link/mongoDatabases",
			TemplatePath: "radius.azurecr.io/recipes/mongodatabases/azure:1.0",
		},
	}
	return recipes, nil
}

func TestCreateOrUpdateRunDevRecipes(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	t.Run("Add dev recipes successfully", func(t *testing.T) {
		getDevRecipes = mockgetDevRecipes
		envInput, envDataModel, expectedOutput := getTestModelsWithDevRecipes20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := testutil.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})
		mStorageClient.
			EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			})

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		mStorageClient.
			EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
				obj.ETag = "new-resource-etag"
				obj.Data = envDataModel
				return nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewCreateOrUpdateEnvironment(opts)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		actualOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})

	t.Run("Append dev recipes to user recipes successfully", func(t *testing.T) {
		envInput, envDataModel, expectedOutput := getTestModelsAppendDevRecipes20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := testutil.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})
		mStorageClient.
			EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			})

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		mStorageClient.
			EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
				obj.ETag = "new-resource-etag"
				obj.Data = envDataModel
				return nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewCreateOrUpdateEnvironment(opts)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		actualOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})

	t.Run("Append dev recipes and user recipes to existing user recipes successfully", func(t *testing.T) {
		envExistingDataModel, envInput, envDataModel, expectedOutput := getTestModelsAppendDevRecipesToExisting20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := testutil.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (res *store.Object, err error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id, ETag: "existing-data-model"},
					Data:     envExistingDataModel,
				}, nil
			})
		mStorageClient.
			EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			})

		mStorageClient.
			EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
				obj.ETag = "new-resource-etag"
				obj.Data = envDataModel
				return nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewCreateOrUpdateEnvironment(opts)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		actualOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})

	t.Run("User recipes conflict with dev recipe names", func(t *testing.T) {
		envInput := getTestModelsUserRecipesConflictWithReservedNames20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := testutil.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewCreateOrUpdateEnvironment(opts)
		require.NoError(t, err)
		_, err = ctl.Run(ctx, w, req)
		require.ErrorContains(
			t,
			err,
			"recipe name(s) reserved for devRecipes for: recipe with name mongo-azure (linkType Applications.Link/mongoDatabases and templatePath radiusdev.azurecr.io/mongo:1.0)")
	})
	t.Run("test input recipes that has dev recipes", func(t *testing.T) {
		envInput := &v20220315privatepreview.EnvironmentResource{
			Location: to.Ptr("West US"),
			Properties: &v20220315privatepreview.EnvironmentProperties{
				Compute: &v20220315privatepreview.KubernetesCompute{
					Kind:       to.Ptr("kubernetes"),
					ResourceID: to.Ptr("fakeid"),
					Namespace:  to.Ptr("default"),
				},
				UseDevRecipes: to.Ptr(true),
				Providers: &v20220315privatepreview.Providers{
					Azure: &v20220315privatepreview.ProvidersAzure{
						Scope: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg"),
					},
				},
				Recipes: map[string]*v20220315privatepreview.EnvironmentRecipeProperties{
					"redis": {
						LinkType:     to.Ptr("Applications.Link/redisCache"),
						TemplatePath: to.Ptr("radiusdev.azurecr.io/redis:1.0"),
					},
					"mongo-azure": {
						LinkType:     to.Ptr("Applications.Link/mongoDatabases"),
						TemplatePath: to.Ptr("radius.azurecr.io/recipes/mongodatabases/azure:1.0"),
					},
					"redis-kubernetes": {
						LinkType:     to.Ptr("Applications.Link/redisCaches"),
						TemplatePath: to.Ptr("radius.azurecr.io/recipes/rediscaches/kubernetes:1.0"),
					},
				},
			},
		}
		rawExpectedOutput := testutil.ReadFixture("environmentappenddevrecipes20220315privatepreview_output.json")
		expectedOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

		rawDataModel := testutil.ReadFixture("environmentappenddevrecipes20220315privatepreview_datamodel.json")
		envDataModel := &datamodel.Environment{}
		_ = json.Unmarshal(rawDataModel, envDataModel)

		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := testutil.ARMTestContextFromRequest(req)

		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{}
			})
		mStorageClient.
			EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, query store.Query, options ...store.QueryOptions) (*store.ObjectQueryResult, error) {
				return &store.ObjectQueryResult{
					Items: []store.Object{},
				}, nil
			})
		mStorageClient.
			EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
				obj.ETag = "new-resource-etag"
				obj.Data = envDataModel
				return nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewCreateOrUpdateEnvironment(opts)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		actualOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})

	t.Run("Existing user recipe conflicts with dev recipe names ", func(t *testing.T) {
		envExistingDataModel, envInput := getTestModelsExistingUserRecipesConflictWithReservedNames20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := testutil.ARMTestContextFromRequest(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (res *store.Object, err error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id, ETag: "existing-data-model"},
					Data:     envExistingDataModel,
				}, nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewCreateOrUpdateEnvironment(opts)
		require.NoError(t, err)
		_, err = ctl.Run(ctx, w, req)
		require.ErrorContains(
			t,
			err,
			"recipe name(s) reserved for devRecipes for: recipe with name mongo-azure (linkType Applications.Link/mongoDatabases and templatePath radiusdev.azurecr.io/mongo:1.0)")
	})

}

// Commenting the test as getDevRecipes fetches the recipes from the acr and compares it to the expectedRecipes.
// If a new recipe is added to the acr the test will fail.
/*func TestGetDevRecipes(t *testing.T) {
	t.Run("Successfully returns dev recipes", func(t *testing.T) {
		ctx := context.Background()
		devRecipes, err := getDevRecipes(ctx)
		require.NoError(t, err)
		expectedRecipes := map[string]datamodel.EnvironmentRecipeProperties{
			"mongo-azure": {
				LinkType:     linkrp.MongoDatabasesResourceType,
				TemplatePath: "radius.azurecr.io/recipes/mongodatabases/azure:1.0",
			},
			"redis-kubernetes": {
				LinkType:     linkrp.RedisCachesResourceType,
				TemplatePath: "radius.azurecr.io/recipes/rediscaches/kubernetes:1.0",
			},
		}
		require.Equal(t, devRecipes, expectedRecipes)
	})
}*/

func TestParseRepoPathForMetadata(t *testing.T) {
	t.Run("Successfully returns metadata", func(t *testing.T) {
		link, provider := parseRepoPathForMetadata("recipes/linkName/providerName")
		require.Equal(t, "linkName", link)
		require.Equal(t, "providerName", provider)
	})

	tests := []struct {
		name             string
		repo             string
		expectedLink     string
		expectedProvider string
	}{
		{
			"Repo isn't related to recipes",
			"randomRepo",
			"",
			"",
		},
		{
			"Repo for recipes doesn't have link and provider names",
			"recipes/noLinkAndProvider",
			"",
			"",
		},
		{
			"Repo for recipes has extra path component",
			"recipes/link/provider/randomValue",
			"",
			"",
		},
		{
			"Repo name has a link and no provider",
			"recipes/linkName/",
			"linkName",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link, provider := parseRepoPathForMetadata(tt.repo)
			require.Equal(t, tt.expectedLink, link)
			require.Equal(t, tt.expectedProvider, provider)
		})
	}
}

func TestFindHighestVersion(t *testing.T) {
	t.Run("Max version is returned when tags are int/float values with float max", func(t *testing.T) {
		versions := []string{"1", "2", "3", "4.0"}
		max, err := findHighestVersion(versions)
		require.NoError(t, err)
		require.Equal(t, max, 4.0)
	})
	t.Run("Max version is returned when tags are int/float values with int max", func(t *testing.T) {
		versions := []string{"1.0", "2.0", "3.0", "4"}
		max, err := findHighestVersion(versions)
		require.NoError(t, err)
		require.Equal(t, max, 4.0)
	})
	t.Run("Version tags are not all float values", func(t *testing.T) {
		versions := []string{"1.0", "otherTag", "3.0", "4.0"}
		_, err := findHighestVersion(versions)
		require.ErrorContains(t, err, "unable to convert tag otherTag into valid version.")
	})
}
