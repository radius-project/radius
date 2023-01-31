// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestGetRecipeDetailsRun_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	ctx := context.Background()
	t.Run("get recipe details run", func(t *testing.T) {
		envInput, expectedOutput := getTestModelsGetRecipeDetails20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := testutil.ARMTestContextFromRequest(req)
		ctl, err := NewGetRecipDetailse(ctrl.Options{})
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		// Set system data to be new and empty.
		expectedOutput.SystemData = new(v20220315privatepreview.SystemData)
		expectedOutput.SystemData.LastModifiedAt = new(time.Time)
		expectedOutput.SystemData.LastModifiedBy = new(string)
		expectedOutput.SystemData.LastModifiedByType = new(v20220315privatepreview.CreatedByType)
		expectedOutput.SystemData.CreatedAt = expectedOutput.SystemData.LastModifiedAt
		expectedOutput.SystemData.CreatedBy = expectedOutput.SystemData.LastModifiedBy
		expectedOutput.SystemData.CreatedByType = expectedOutput.SystemData.LastModifiedByType

		actualOutput := &v20220315privatepreview.EnvironmentResource{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
		require.Equal(t, expectedOutput, actualOutput)
	})

	t.Run("get recipe details run with multiple recipes", func(t *testing.T) {
		envInput := getTestModelsGetRecipeDetailsWithMultipleRecipes20220315privatepreview()
		w := httptest.NewRecorder()
		req, _ := testutil.GetARMTestHTTPRequest(ctx, http.MethodGet, testHeaderfile, envInput)
		ctx := testutil.ARMTestContextFromRequest(req)
		ctl, err := NewGetRecipDetailse(ctrl.Options{})
		require.NoError(t, err)
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		_ = resp.Apply(ctx, w, req)
		result := w.Result()
		require.Equal(t, 400, result.StatusCode)

		body := result.Body
		defer body.Close()
		payload, err := io.ReadAll(body)
		require.NoError(t, err)

		armerr := v1.ErrorResponse{}
		err = json.Unmarshal(payload, &armerr)
		require.NoError(t, err)
		require.Equal(t, v1.CodeInvalid, armerr.Error.Code)
		require.Equal(t, "Only one recipe should be specified in the request.", armerr.Error.Message)
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
