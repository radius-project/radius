// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestGetRecipeDetailsRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()
	mStorageClient := store.NewMockStorageClient(mctrl)
	ctx := context.Background()

	t.Parallel()
	t.Run("get recipe details run", func(t *testing.T) {
		envDataModel, expectedOutput := getTestModelsGetRecipeDetails20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, OperationGetRecipeDetails, testHeaderfilegetrecipedetails, nil)

		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id, ETag: "etag"},
					Data:     envDataModel,
				}, nil
			})
		ctx := testutil.ARMTestContextFromRequest(req)

		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}
		ctl, err := NewGetRecipDetails(opts)
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})

	t.Run("get recipe details run non existing environment", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, OperationGetRecipeDetails, testHeaderfilegetrecipedetails, nil)
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
		ctl, err := NewGetRecipDetails(opts)
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

	t.Run("get recipe details non existing recipe", func(t *testing.T) {
		envDataModel, _ := getTestModelsGetRecipeDetails20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, OperationGetRecipeDetails, testHeaderfilegetrecipedetailsnotexisting, nil)
		ctx := testutil.ARMTestContextFromRequest(req)

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
		}
		ctl, err := NewGetRecipDetails(opts)
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
		require.Contains(t, armerr.Error.Message, "Recipe with name \"mongodb\" not found on environment with id")
	})
}

func TestGetRecipeDetailsFromRegistry(t *testing.T) {
	ctx := context.Background()

	t.Run("get recipe details from registry", func(t *testing.T) {
		recipeDetails := datamodel.EnvironmentRecipeProperties{
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
		}
		err := GetRecipeDetailsFromRegistry(ctx, &recipeDetails, "mongodb")
		require.NoError(t, err)
		expectedOutput := map[string]any{
			"mongodbName":    "type : string\t",
			"documentdbName": "type : string\t",
			"location":       "type : string\tdefaultValue : [resourceGroup().location]\t",
		}
		require.Equal(t, expectedOutput, recipeDetails.Parameters)
	})

	t.Run("get recipe details from registry with context parameter", func(t *testing.T) {
		recipeDetails := datamodel.EnvironmentRecipeProperties{
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/mongodatabases/azure:1.0",
		}
		err := GetRecipeDetailsFromRegistry(ctx, &recipeDetails, "mongodb")
		require.NoError(t, err)
		expectedOutput := map[string]any{
			"location": "type : string\tdefaultValue : [resourceGroup().location]\t",
		}
		require.Equal(t, expectedOutput, recipeDetails.Parameters)
	})

	t.Run("get recipe details from registry with invalid path", func(t *testing.T) {
		recipeDetails := datamodel.EnvironmentRecipeProperties{
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/test/mongodatabases/azure:1.0",
		}
		err := GetRecipeDetailsFromRegistry(ctx, &recipeDetails, "mongodb")
		require.Error(t, err, "failed to fetch template from the path \"radiusdev.azurecr.io/recipes/functionaltest/test/mongodatabases/azure:1.0\" for recipe \"mongodb\": radiusdev.azurecr.io/recipes/functionaltest/test/mongodatabases/azure:1.0: not found")
	})
}
