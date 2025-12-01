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
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestRecipePackConvertVersionedToDataModel(t *testing.T) {
	// Load test data
	data := testutil.ReadFixture("recipepackresource.json")

	// Unmarshal into versioned resource
	var versionedResource RecipePackResource
	err := json.Unmarshal(data, &versionedResource)
	require.NoError(t, err)

	// Convert to data model
	dm, err := versionedResource.ConvertTo()
	require.NoError(t, err)
	require.NotNil(t, dm)

	// Verify it's the right type
	recipePack, ok := dm.(*datamodel.RecipePack)
	require.True(t, ok)

	// Basic validations
	require.Equal(t, *versionedResource.ID, recipePack.ID)
	require.Equal(t, *versionedResource.Name, recipePack.Name)
	require.Equal(t, *versionedResource.Type, recipePack.Type)
	require.Equal(t, *versionedResource.Location, recipePack.Location)

	// Validate API version metadata
	require.Equal(t, Version, recipePack.InternalMetadata.CreatedAPIVersion)
	require.Equal(t, Version, recipePack.InternalMetadata.UpdatedAPIVersion)
}

func TestRecipePackConvertDataModelToVersioned(t *testing.T) {
	// Load test data
	data := testutil.ReadFixture("recipepackresourcedatamodel.json")

	// Unmarshal into datamodel
	var dataModel datamodel.RecipePack
	err := json.Unmarshal(data, &dataModel)
	require.NoError(t, err)

	// Convert to versioned resource
	var versionedResource RecipePackResource
	err = versionedResource.ConvertFrom(&dataModel)
	require.NoError(t, err)

	// Basic validations
	require.Equal(t, dataModel.ID, *versionedResource.ID)
	require.Equal(t, dataModel.Name, *versionedResource.Name)
	require.Equal(t, dataModel.Type, *versionedResource.Type)
	require.Equal(t, dataModel.Location, *versionedResource.Location)
	require.NotNil(t, versionedResource.Properties)
}

func TestRecipePackConvertInvalidModel(t *testing.T) {
	t.Run("invalid model type", func(t *testing.T) {
		var versionedResource RecipePackResource

		// Try to convert from wrong model type
		invalidModel := &datamodel.Environment{}
		err := versionedResource.ConvertFrom(invalidModel)
		require.Error(t, err)
		require.Equal(t, v1.ErrInvalidModelConversion, err)
	})
}
