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

package recipepacks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
)

func TestNewCreateOrUpdateRecipePack(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	controller, err := NewCreateOrUpdateRecipePack(opts)
	require.NoError(t, err)
	require.NotNil(t, controller)
}

func TestCreateOrUpdateRecipePackRun_CreateNew(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)

	recipePackInput, recipePackDataModel, expectedOutput := getTestModels()
	w := httptest.NewRecorder()

	jsonPayload, err := json.Marshal(recipePackInput)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/default/providers/Radius.Core/recipePacks/testrecipepack?api-version=2025-08-01-preview", strings.NewReader(string(jsonPayload)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	ctx := rpctest.NewARMRequestContext(req)

	databaseClient.
		EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return nil, &database.ErrNotFound{ID: id}
		})

	databaseClient.
		EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
			obj.ETag = "new-resource-etag"
			obj.Data = recipePackDataModel
			return nil
		})

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	ctl, err := NewCreateOrUpdateRecipePack(opts)
	require.NoError(t, err)
	resp, err := ctl.Run(ctx, w, req)
	require.NoError(t, err)
	_ = resp.Apply(ctx, w, req)
	require.Equal(t, 200, w.Result().StatusCode)

	actualOutput := &v20250801preview.RecipePackResource{}
	_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
	require.Equal(t, expectedOutput.Properties.Recipes, actualOutput.Properties.Recipes)
	require.Equal(t, v20250801preview.ProvisioningStateSucceeded, *actualOutput.Properties.ProvisioningState)
}

func TestCreateOrUpdateRecipePackRun_UpdateExisting(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	recipePackInput, recipePackDataModel, expectedOutput := getTestModels()
	w := httptest.NewRecorder()

	jsonPayload, err := json.Marshal(recipePackInput)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/default/providers/Radius.Core/recipePacks/testrecipepack?api-version=2025-08-01-preview", strings.NewReader(string(jsonPayload)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	ctx := rpctest.NewARMRequestContext(req)

	databaseClient.
		EXPECT().
		Get(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return &database.Object{
				Data: recipePackDataModel,
			}, nil
		})

	databaseClient.
		EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
			obj.Data = recipePackDataModel
			return nil
		})

	opts := ctrl.Options{
		DatabaseClient: databaseClient,
	}

	ctl, err := NewCreateOrUpdateRecipePack(opts)
	require.NoError(t, err)
	resp, err := ctl.Run(ctx, w, req)
	require.NoError(t, err)
	_ = resp.Apply(ctx, w, req)
	require.Equal(t, 200, w.Result().StatusCode)

	actualOutput := &v20250801preview.RecipePackResource{}
	_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
	require.Equal(t, expectedOutput.Properties.Recipes, actualOutput.Properties.Recipes)
	require.Equal(t, v20250801preview.ProvisioningStateSucceeded, *actualOutput.Properties.ProvisioningState)
}

func getTestModels() (*v20250801preview.RecipePackResource, *datamodel.RecipePack, *v20250801preview.RecipePackResource) {
	resourceID := "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/default/providers/Radius.Core/recipePacks/testrecipepack"
	resourceName := "testrecipepack"
	location := "global"

	recipePackInput := &v20250801preview.RecipePackResource{
		Location: &location,
		Properties: &v20250801preview.RecipePackProperties{
			Recipes: map[string]*v20250801preview.RecipeDefinition{
				"Applications.Core/extenders": {
					RecipeKind:     to.Ptr(v20250801preview.RecipeKindBicep),
					RecipeLocation: to.Ptr("ghcr.io/radius-project/recipes/local-dev/extender-postgresql:0.50.0"),
				},
				"Radius.Resources/postgreSQL": {
					RecipeKind:     to.Ptr(v20250801preview.RecipeKindBicep),
					RecipeLocation: to.Ptr("ghcr.io/radius-project/recipes/local-dev/extender-postgresql:0.50.0"),
				},
				"Applications.Datastores/redisCaches": {
					RecipeKind:     to.Ptr(v20250801preview.RecipeKindBicep),
					RecipeLocation: to.Ptr("https://github.com/example/recipes/redis-cache.bicep"),
					Parameters: map[string]any{
						"tier": "basic",
					},
				},
			},
		},
	}

	recipePackDataModel := &datamodel.RecipePack{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       resourceID,
				Name:     resourceName,
				Type:     datamodel.RecipePackResourceType,
				Location: location,
			},
		},
		Properties: datamodel.RecipePackProperties{
			Recipes: map[string]*datamodel.RecipeDefinition{
				"Applications.Core/extenders": {
					RecipeKind:     "bicep",
					RecipeLocation: "ghcr.io/radius-project/recipes/local-dev/extender-postgresql:0.50.0",
				},
				"Radius.Resources/postgreSQL": {
					RecipeKind:     "bicep",
					RecipeLocation: "ghcr.io/radius-project/recipes/local-dev/extender-postgresql:0.50.0",
				},
				"Applications.Datastores/redisCaches": {
					RecipeKind:     "bicep",
					RecipeLocation: "https://github.com/example/recipes/redis-cache.bicep",
					Parameters: map[string]any{
						"tier": "basic",
					},
				},
			},
		},
	}

	expectedOutput := &v20250801preview.RecipePackResource{
		ID:       &resourceID,
		Name:     &resourceName,
		Type:     to.Ptr(datamodel.RecipePackResourceType),
		Location: &location,
		Properties: &v20250801preview.RecipePackProperties{
			ProvisioningState: to.Ptr(v20250801preview.ProvisioningStateSucceeded),
			Recipes: map[string]*v20250801preview.RecipeDefinition{
				"Applications.Core/extenders": {
					RecipeKind:     to.Ptr(v20250801preview.RecipeKindBicep),
					RecipeLocation: to.Ptr("ghcr.io/radius-project/recipes/local-dev/extender-postgresql:0.50.0"),
					PlainHTTP:      to.Ptr(false),
				},
				"Radius.Resources/postgreSQL": {
					RecipeKind:     to.Ptr(v20250801preview.RecipeKindBicep),
					RecipeLocation: to.Ptr("ghcr.io/radius-project/recipes/local-dev/extender-postgresql:0.50.0"),
					PlainHTTP:      to.Ptr(false),
				},
				"Applications.Datastores/redisCaches": {
					RecipeKind:     to.Ptr(v20250801preview.RecipeKindBicep),
					RecipeLocation: to.Ptr("https://github.com/example/recipes/redis-cache.bicep"),
					Parameters: map[string]any{
						"tier": "basic",
					},
					PlainHTTP: to.Ptr(false),
				},
			},
		},
	}

	return recipePackInput, recipePackDataModel, expectedOutput
}
