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

package v20250801preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/test/k8sutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// testRecipePackID is the recipe pack referenced by the shared environment test
// fixtures (environmentresource_input.json). The run test mocks its existence so
// the controller's recipe pack validation passes, mirroring how the test also
// provisions the "default" namespace the controller validates.
const testRecipePackID = "/planes/radius/local/providers/Radius.Core/recipePacks/kubernetes-pack"

// testRecipePackObject returns a minimal valid recipe pack stored object. Its
// Recipes map is empty so a single referenced pack never triggers a cross-pack
// resource-type conflict.
func testRecipePackObject() *database.Object {
	return &database.Object{
		Data: &datamodel.RecipePack{
			Properties: datamodel.RecipePackProperties{
				Recipes: map[string]*datamodel.RecipeDefinition{},
			},
		},
	}
}

func TestCreateOrUpdateEnvironmentRun_20250801Preview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	ctx := context.Background()

	// The shared environment fixtures reference a recipe pack by ID; mock its
	// existence for every subtest so the controller's recipe pack validation
	// passes. Matching the specific ID prevents this from shadowing the
	// per-subtest environment Get expectations.
	databaseClient.EXPECT().
		Get(gomock.Any(), testRecipePackID).
		Return(testRecipePackObject(), nil).
		AnyTimes()

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
			envInput, envDataModel, expectedOutput := getTestModelsv20250801preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfilev20250801preview, envInput)
			require.NoError(t, err)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := rpctest.NewARMRequestContext(req)

			databaseClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return nil, &database.ErrNotFound{ID: id}
				})

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
						return &database.ObjectQueryResult{
							Items: []database.Object{},
						}, nil
					})
			}

			expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
			expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
			expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
						obj.ETag = "new-resource-etag"
						obj.Data = envDataModel
						return nil
					})
			}

			defaultNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil, defaultNamespace),
			}

			ctl, err := NewCreateOrUpdateEnvironmentv20250801preview(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)
			if !tt.shouldFail {
				actualOutput := &v20250801preview.EnvironmentResource{}
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
			envInput, envDataModel, expectedOutput := getTestModelsv20250801preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodGet, testHeaderfilev20250801preview, envInput)
			require.NoError(t, err)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := rpctest.NewARMRequestContext(req)

			databaseClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return &database.Object{
						Metadata: database.Metadata{ID: id, ETag: tt.resourceETag},
						Data:     envDataModel,
					}, nil
				})

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
						return &database.ObjectQueryResult{
							Items: []database.Object{},
						}, nil
					})
			}

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
						obj.ETag = "updated-resource-etag"
						obj.Data = envDataModel
						return nil
					})
			}

			defaultNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil, defaultNamespace),
			}

			ctl, err := NewCreateOrUpdateEnvironmentv20250801preview(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)

			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20250801preview.EnvironmentResource{}
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
			envInput, _, _ := getTestModelsv20250801preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodPatch, testHeaderfilev20250801preview, envInput)
			require.NoError(t, err)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := rpctest.NewARMRequestContext(req)

			databaseClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return nil, &database.ErrNotFound{ID: id}
				})

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
						return &database.ObjectQueryResult{
							Items: []database.Object{},
						}, nil
					})
			}

			defaultNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil, defaultNamespace),
			}

			ctl, err := NewCreateOrUpdateEnvironmentv20250801preview(opts)
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
			envInput, envDataModel, expectedOutput := getTestModelsv20250801preview()
			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodPatch, testHeaderfilev20250801preview, envInput)
			require.NoError(t, err)
			req.Header.Set(tt.headerKey, tt.headerValue)
			ctx := rpctest.NewARMRequestContext(req)

			databaseClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					return &database.Object{
						Metadata: database.Metadata{ID: id, ETag: tt.resourceEtag},
						Data:     envDataModel,
					}, nil
				})

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Query(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
						return &database.ObjectQueryResult{
							Items: []database.Object{},
						}, nil
					})
			}

			if !tt.shouldFail {
				databaseClient.
					EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
						cfg := database.NewSaveConfig(opts...)
						obj.ETag = cfg.ETag
						obj.Data = envDataModel
						return nil
					})
			}

			defaultNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil, defaultNamespace),
			}

			ctl, err := NewCreateOrUpdateEnvironmentv20250801preview(opts)
			require.NoError(t, err)
			resp, err := ctl.Run(ctx, w, req)
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if !tt.shouldFail {
				actualOutput := &v20250801preview.EnvironmentResource{}
				_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
				require.Equal(t, expectedOutput, actualOutput)
			}
		})
	}
}

func TestCreateOrUpdateEnvironment_RecipePackValidation(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		desc               string
		recipePacks        []string
		setupMockDB        func(*database.MockClient)
		expectedStatusCode int
		expectedError      string
		expectedRunError   string
	}{
		{
			desc:        "single-recipe-pack-validated",
			recipePacks: []string{"/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1"},
			setupMockDB: func(databaseClient *database.MockClient) {
				pack1 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Core/containers": {
								RecipeKind:     "bicep",
								RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							},
						},
					},
				}

				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1").
					Return(&database.Object{Data: pack1}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			desc:        "valid-multiple-recipe-packs-no-conflicts",
			recipePacks: []string{"/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1", "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack2"},
			setupMockDB: func(databaseClient *database.MockClient) {
				pack1 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Core/containers": {
								RecipeKind:     "bicep",
								RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							},
						},
					},
				}
				pack2 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Dapr/stateStores": {
								RecipeKind:     "terraform",
								RecipeLocation: "git::https://github.com/recipes/dapr-state",
							},
						},
					},
				}

				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1").
					Return(&database.Object{Data: pack1}, nil)

				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack2").
					Return(&database.Object{Data: pack2}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			desc:        "duplicate-same-recipe-pack-by-name-and-id-no-conflict",
			recipePacks: []string{"myPack", "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/recipePacks/myPack"},
			setupMockDB: func(databaseClient *database.MockClient) {
				pack := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Core/containers": {
								RecipeKind:     "bicep",
								RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							},
						},
					},
				}

				databaseClient.EXPECT().
					Get(gomock.Any(), "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/recipePacks/myPack").
					Return(&database.Object{Data: pack}, nil).
					Times(2)
			},
			expectedStatusCode: 200,
		},
		{
			desc:        "conflicting-recipe-packs-same-resource-type",
			recipePacks: []string{"/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1", "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack2"},
			setupMockDB: func(databaseClient *database.MockClient) {
				pack1 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Core/containers": {
								RecipeKind:     "bicep",
								RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							},
						},
					},
				}
				pack2 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Core/containers": {
								RecipeKind:     "terraform",
								RecipeLocation: "git::https://github.com/recipes/container",
							},
						},
					},
				}

				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1").
					Return(&database.Object{Data: pack1}, nil)

				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack2").
					Return(&database.Object{Data: pack2}, nil)
			},
			expectedStatusCode: 409,
			expectedError:      "Resource type 'Applications.Core/containers' is defined in multiple recipe packs",
		},
		{
			desc:        "conflicting-recipe-packs-same-resource-type-different-case",
			recipePacks: []string{"/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1", "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack2"},
			setupMockDB: func(databaseClient *database.MockClient) {
				pack1 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Core/containers": {
								RecipeKind:     "bicep",
								RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							},
						},
					},
				}
				pack2 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"applications.core/containers": {
								RecipeKind:     "terraform",
								RecipeLocation: "git::https://github.com/recipes/container",
							},
						},
					},
				}

				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1").
					Return(&database.Object{Data: pack1}, nil)

				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack2").
					Return(&database.Object{Data: pack2}, nil)
			},
			expectedStatusCode: 409,
			expectedError:      "is defined in multiple recipe packs",
		},
		{
			desc:        "non-existent-recipe-pack",
			recipePacks: []string{"/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1", "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/nonexistent"},
			setupMockDB: func(databaseClient *database.MockClient) {
				pack1 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Core/containers": {
								RecipeKind:     "bicep",
								RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							},
						},
					},
				}

				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1").
					Return(&database.Object{Data: pack1}, nil)

				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/nonexistent").
					Return(nil, &database.ErrNotFound{ID: "nonexistent"})
			},
			expectedStatusCode: 400,
			expectedError:      "could not be found",
		},
		{
			desc:        "recipe-pack-by-name-resolved",
			recipePacks: []string{"myPack"},
			setupMockDB: func(databaseClient *database.MockClient) {
				pack := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Core/containers": {
								RecipeKind:     "bicep",
								RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							},
						},
					},
				}

				databaseClient.EXPECT().
					Get(gomock.Any(), "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/recipePacks/myPack").
					Return(&database.Object{Data: pack}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			desc:        "recipe-pack-by-name-not-found",
			recipePacks: []string{"ghost"},
			setupMockDB: func(databaseClient *database.MockClient) {
				databaseClient.EXPECT().
					Get(gomock.Any(), "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/recipePacks/ghost").
					Return(nil, &database.ErrNotFound{ID: "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/recipePacks/ghost"})
			},
			expectedStatusCode: 400,
			expectedError:      "could not be found",
		},
		{
			desc:        "recipe-pack-lookup-error",
			recipePacks: []string{"/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1"},
			setupMockDB: func(databaseClient *database.MockClient) {
				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack1").
					Return(nil, errors.New("database unavailable"))
			},
			expectedRunError: "database unavailable",
		},
		{
			desc:        "mixed-name-and-full-id",
			recipePacks: []string{"myPack", "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack2"},
			setupMockDB: func(databaseClient *database.MockClient) {
				pack1 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Core/containers": {
								RecipeKind:     "bicep",
								RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
							},
						},
					},
				}
				pack2 := &datamodel.RecipePack{
					Properties: datamodel.RecipePackProperties{
						Recipes: map[string]*datamodel.RecipeDefinition{
							"Applications.Dapr/stateStores": {
								RecipeKind:     "terraform",
								RecipeLocation: "git::https://github.com/recipes/dapr-state",
							},
						},
					},
				}

				databaseClient.EXPECT().
					Get(gomock.Any(), "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/recipePacks/myPack").
					Return(&database.Object{Data: pack1}, nil)
				databaseClient.EXPECT().
					Get(gomock.Any(), "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack2").
					Return(&database.Object{Data: pack2}, nil)
			},
			expectedStatusCode: 200,
		},
		{
			desc:               "wrong-type-recipe-pack-id",
			recipePacks:        []string{"/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/terraformConfigs/tc"},
			setupMockDB:        func(*database.MockClient) {},
			expectedStatusCode: 400,
			expectedError:      "has type",
		},
		{
			desc:               "invalid-recipe-pack-name",
			recipePacks:        []string{"a/b"},
			setupMockDB:        func(*database.MockClient) {},
			expectedStatusCode: 400,
			expectedError:      "Invalid recipe pack reference",
		},
		{
			desc:               "invalid-recipe-pack-name-leading-slash",
			recipePacks:        []string{"/pack"},
			setupMockDB:        func(*database.MockClient) {},
			expectedStatusCode: 400,
			expectedError:      "Invalid recipe pack reference",
		},
		{
			desc:               "invalid-empty-recipe-pack-name",
			recipePacks:        []string{""},
			setupMockDB:        func(*database.MockClient) {},
			expectedStatusCode: 400,
			expectedError:      "Invalid recipe pack reference",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			// Create fresh mock for each test
			mctrl := gomock.NewController(t)
			defer mctrl.Finish()
			databaseClient := database.NewMockClient(mctrl)

			envInput, envDataModel, _ := getTestModelsv20250801preview()

			// Convert []string to []*string for API model
			recipePacks := make([]*string, len(tt.recipePacks))
			for i, rp := range tt.recipePacks {
				rpCopy := rp
				recipePacks[i] = &rpCopy
			}
			envInput.Properties.RecipePacks = recipePacks

			w := httptest.NewRecorder()
			req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodPut, testHeaderfilev20250801preview, envInput)
			require.NoError(t, err)
			ctx := rpctest.NewARMRequestContext(req)

			// Setup recipe pack mocks first (they are called during validation)
			tt.setupMockDB(databaseClient)

			// Mock the environment resource lookup (not found for create scenario) - this happens first
			databaseClient.EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
					// If it's a recipe pack resource, don't handle it here (let setupMockDB handle it)
					if strings.Contains(id, "recipePacks") {
						panic("Recipe pack Get should be handled by setupMockDB")
					}
					return nil, &database.ErrNotFound{ID: id}
				}).AnyTimes()

			// Mock kubernetes namespace query - this happens before recipe pack validation
			databaseClient.EXPECT().
				Query(gomock.Any(), gomock.Any()).
				Return(&database.ObjectQueryResult{Items: []database.Object{}}, nil).MaxTimes(1)

			// Mock Save only for successful cases
			if tt.expectedStatusCode == 200 {
				databaseClient.EXPECT().
					Save(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
						obj.ETag = "new-resource-etag"
						obj.Data = envDataModel
						return nil
					}).MaxTimes(1)
			}

			defaultNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
			opts := ctrl.Options{
				DatabaseClient: databaseClient,
				KubeClient:     k8sutil.NewFakeKubeClient(nil, defaultNamespace),
			}

			ctl, err := NewCreateOrUpdateEnvironmentv20250801preview(opts)
			require.NoError(t, err)

			resp, err := ctl.Run(ctx, w, req)
			if tt.expectedRunError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedRunError)
				require.Nil(t, resp)
				return
			}
			require.NoError(t, err)
			_ = resp.Apply(ctx, w, req)

			require.Equal(t, tt.expectedStatusCode, w.Result().StatusCode)

			if tt.expectedError != "" {
				require.Contains(t, w.Body.String(), tt.expectedError)
			}
		})
	}
}

// TestCreateOrUpdateEnvironment_RecipePackNormalization verifies that recipe pack
// references are normalized to canonical full resource IDs before being persisted: a
// bare name is resolved against the environment's own plane and resource group, while a
// full resource ID is stored unchanged.
func TestCreateOrUpdateEnvironment_RecipePackNormalization(t *testing.T) {
	ctx := context.Background()

	const (
		envID          = "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/environments/my-k8s-env"
		resolvedByName = "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/recipePacks/myPack"
		fullID         = "/subscriptions/sub1/resourceGroups/rg1/providers/Radius.Core/recipePacks/pack2"
	)

	mctrl := gomock.NewController(t)
	defer mctrl.Finish()
	databaseClient := database.NewMockClient(mctrl)

	envInput, _, _ := getTestModelsv20250801preview()

	// Reference one pack by bare name and one by full resource ID.
	refs := []string{"myPack", fullID}
	recipePacks := make([]*string, len(refs))
	for i := range refs {
		recipePacks[i] = &refs[i]
	}
	envInput.Properties.RecipePacks = recipePacks

	w := httptest.NewRecorder()
	req, err := rpctest.NewHTTPRequestFromJSON(ctx, http.MethodPut, testHeaderfilev20250801preview, envInput)
	require.NoError(t, err)
	ctx = rpctest.NewARMRequestContext(req)

	packByName := &datamodel.RecipePack{
		Properties: datamodel.RecipePackProperties{
			Recipes: map[string]*datamodel.RecipeDefinition{
				"Applications.Core/containers": {
					RecipeKind:     "bicep",
					RecipeLocation: "br:myregistry.azurecr.io/recipes/container:1.0",
				},
			},
		},
	}
	packByID := &datamodel.RecipePack{
		Properties: datamodel.RecipePackProperties{
			Recipes: map[string]*datamodel.RecipeDefinition{
				"Applications.Dapr/stateStores": {
					RecipeKind:     "terraform",
					RecipeLocation: "git::https://github.com/recipes/dapr-state",
				},
			},
		},
	}

	// Environment does not yet exist (create scenario).
	databaseClient.EXPECT().
		Get(gomock.Any(), envID).
		Return(nil, &database.ErrNotFound{ID: envID})
	// The bare name resolves to the environment's plane + resource group.
	databaseClient.EXPECT().
		Get(gomock.Any(), resolvedByName).
		Return(&database.Object{Data: packByName}, nil)
	// The full ID is looked up as-is.
	databaseClient.EXPECT().
		Get(gomock.Any(), fullID).
		Return(&database.Object{Data: packByID}, nil)

	databaseClient.EXPECT().
		Query(gomock.Any(), gomock.Any()).
		Return(&database.ObjectQueryResult{Items: []database.Object{}}, nil).MaxTimes(1)

	var savedRecipePacks []string
	databaseClient.EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
			env, ok := obj.Data.(*datamodel.Environment_v20250801preview)
			require.True(t, ok, "saved object should be an Environment_v20250801preview")
			savedRecipePacks = env.Properties.RecipePacks
			obj.ETag = "new-resource-etag"
			return nil
		})

	defaultNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	opts := ctrl.Options{
		DatabaseClient: databaseClient,
		KubeClient:     k8sutil.NewFakeKubeClient(nil, defaultNamespace),
	}

	ctl, err := NewCreateOrUpdateEnvironmentv20250801preview(opts)
	require.NoError(t, err)

	resp, err := ctl.Run(ctx, w, req)
	require.NoError(t, err)
	_ = resp.Apply(ctx, w, req)

	require.Equal(t, 200, w.Result().StatusCode)
	require.Equal(t, []string{resolvedByName, fullID}, savedRecipePacks)
}
