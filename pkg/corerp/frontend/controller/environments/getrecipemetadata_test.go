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

package environments

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestGetRecipeMetadataRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()
	mStorageClient := store.NewMockStorageClient(mctrl)
	mEngine := engine.NewMockEngine(mctrl)
	ctx := context.Background()
	t.Parallel()
	t.Run("get recipe metadata run", func(t *testing.T) {
		envInput, envDataModel, expectedOutput := getTestModelsGetRecipeMetadata20220315privatepreview()
		w := httptest.NewRecorder()
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, v1.OperationPost.HTTPMethod(), testHeaderfilegetrecipemetadata, envInput)
		require.NoError(t, err)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id, ETag: "etag"},
					Data:     envDataModel,
				}, nil
			})
		ctx := rpctest.NewARMRequestContext(req)
		recipeMetadata := recipes.ResourceMetadata{
			Name:          *envInput.Name,
			EnvironmentID: envDataModel.ID,
			Parameters:    nil,
			ResourceID:    envDataModel.ID,
			ResourceType:  "Applications.Link/mongoDatabases",
		}
		recipeData := map[string]any{
			"parameters": map[string]any{
				"documentdbName": map[string]any{"type": "string"},
				"location":       map[string]any{"defaultValue": "[resourceGroup().location]", "type": "string"},
				"mongodbName":    map[string]any{"type": "string"},
			},
		}
		mEngine.EXPECT().GetRecipeMetadata(ctx, recipeMetadata).Return(recipeData, nil)

		opts := ctrl.Options{
			StorageClient: mStorageClient,
			Engine:        mEngine,
		}
		ctl, err := NewGetRecipeMetadata(opts)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.RecipeGetMetadataResponse{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})

	t.Run("get recipe metadata run non existing environment", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, v1.OperationPost.HTTPMethod(), testHeaderfilegetrecipemetadata, nil)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return nil, &store.ErrNotFound{ID: id}
			})
		opts := ctrl.Options{
			StorageClient: mStorageClient,
			Engine:        mEngine,
		}
		ctl, err := NewGetRecipeMetadata(opts)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		result := w.Result()
		require.Equal(t, 404, result.StatusCode)

		body := result.Body
		defer body.Close()
		payload, err := io.ReadAll(body)
		require.NoError(t, err)

		armerr := v1.ErrorResponse{}
		err = json.Unmarshal(payload, &armerr)
		require.NoError(t, err)
		require.Equal(t, v1.CodeNotFound, armerr.Error.Code)
		require.Contains(t, armerr.Error.Message, "the resource with id")
		require.Contains(t, armerr.Error.Message, "was not found")
	})

	t.Run("get recipe metadata non existing recipe", func(t *testing.T) {
		envInput, envDataModel := getTestModelsGetRecipeMetadataForNonExistingRecipe20220315privatepreview()
		w := httptest.NewRecorder()
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, v1.OperationPost.HTTPMethod(), testHeaderfilegetrecipemetadatanotexisting, envInput)
		require.NoError(t, err)
		ctx := rpctest.NewARMRequestContext(req)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id, ETag: "etag"},
					Data:     envDataModel,
				}, nil
			})

		opts := ctrl.Options{
			StorageClient: mStorageClient,
			Engine:        mEngine,
		}
		ctl, err := NewGetRecipeMetadata(opts)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		result := w.Result()
		require.Equal(t, 404, result.StatusCode)

		body := result.Body
		defer body.Close()
		payload, err := io.ReadAll(body)
		require.NoError(t, err)

		armerr := v1.ErrorResponse{}
		err = json.Unmarshal(payload, &armerr)
		require.NoError(t, err)
		require.Equal(t, v1.CodeNotFound, armerr.Error.Code)
		require.Contains(t, armerr.Error.Message, "Either recipe with name \"mongodb\" or resource type \"Applications.Datastores/mongoDatabases\" not found on environment with id")
	})

	t.Run("get recipe metadata engine failure", func(t *testing.T) {
		envInput, envDataModel, _ := getTestModelsGetRecipeMetadata20220315privatepreview()
		w := httptest.NewRecorder()
		req, err := rpctest.NewHTTPRequestFromJSON(ctx, v1.OperationPost.HTTPMethod(), testHeaderfilegetrecipemetadata, envInput)
		require.NoError(t, err)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id, ETag: "etag"},
					Data:     envDataModel,
				}, nil
			})
		ctx := rpctest.NewARMRequestContext(req)
		recipeMetadata := recipes.ResourceMetadata{
			Name:          *envInput.Name,
			EnvironmentID: envDataModel.ID,
			Parameters:    nil,
			ResourceID:    envDataModel.ID,
			ResourceType:  "Applications.Link/mongoDatabases",
		}
		engineErr := fmt.Errorf("could not find driver %s", "invalidDriver")
		mEngine.EXPECT().GetRecipeMetadata(ctx, recipeMetadata).Return(nil, engineErr)

		opts := ctrl.Options{
			StorageClient: mStorageClient,
			Engine:        mEngine,
		}
		ctl, err := NewGetRecipeMetadata(opts)
		require.NoError(t, err)
		_, err = ctl.Run(ctx, w, req)
		require.Error(t, err)
		require.Equal(t, err, engineErr)
	})
}

func TestParseAndFormatRecipeParams(t *testing.T) {
	t.Run("parse and format recipe parameters with context", func(t *testing.T) {
		recipeData := map[string]any{}
		recipeDataJSON := testutil.ReadFixture("recipedatawithparameters.json")
		_ = json.Unmarshal(recipeDataJSON, &recipeData)
		output := map[string]any{}
		err := parseAndFormatRecipeParams(recipeData, output)
		require.NoError(t, err)
		expectedOutput := map[string]any{
			"storageAccountName": map[string]any{
				"type": "string",
			},
			"storageAccountType": map[string]any{
				"type": "string",
				"allowedValues": []any{
					"Premium_LRS",
					"Premium_ZRS",
					"Standard_GRS",
					"Standard_GZRS",
					"Standard_LRS",
					"Standard_RAGRS",
					"Standard_RAGZRS",
					"Standard_ZRS",
				},
			},
			"location": map[string]any{
				"type":         "string",
				"defaultValue": "[resourceGroup().location]",
			},
		}

		require.Equal(t, expectedOutput, output)
	})

	t.Run("parse and format recipe with no parameters", func(t *testing.T) {
		recipeData := map[string]any{}
		_ = json.Unmarshal(testutil.ReadFixture("recipedatawithoutparameters.json"), &recipeData)
		output := map[string]any{}
		err := parseAndFormatRecipeParams(recipeData, output)
		require.NoError(t, err)
		expectedOutput := map[string]any{}
		require.Equal(t, expectedOutput, output)
	})

	t.Run("parse and format recipe with malformed parameters", func(t *testing.T) {
		recipeData := map[string]any{}
		_ = json.Unmarshal(testutil.ReadFixture("recipedatawithmalformedparameters.json"), &recipeData)
		err := parseAndFormatRecipeParams(recipeData, map[string]any{})
		require.ErrorContains(t, err, "parameters are not in expected format")
	})

	t.Run("parse and format recipe with malformed parameter details", func(t *testing.T) {
		recipeData := map[string]any{}
		_ = json.Unmarshal(testutil.ReadFixture("recipedatawithmalformedparameterdetails.json"), &recipeData)
		err := parseAndFormatRecipeParams(recipeData, map[string]any{})
		require.ErrorContains(t, err, "parameter details are not in expected format")
	})
}
