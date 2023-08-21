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

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/linkrp"
	types "github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func TestEnvironmentRecipePropertiesConvertVersionedToDataModel(t *testing.T) {
	t.Run("Convert to Data Model", func(t *testing.T) {
		r := &RecipeMetadataProperties{}
		// act
		_, err := r.ConvertTo()

		require.ErrorContains(t, err, "converting Environment Recipe Properties to a version-agnostic object is not supported")
	})
}

func TestEnvironmentRecipePropertiesConvertDataModelToVersioned(t *testing.T) {

	files := []string{"environmentrecipepropertiesdatamodel.json", "environmentrecipepropertiesdatamodel-terraform.json"}
	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(filename)
			r := &datamodel.EnvironmentRecipeProperties{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			versioned := &RecipeMetadataProperties{}
			err = versioned.ConvertFrom(r)
			// assert
			require.NoError(t, err)
			require.Equal(t, r.TemplatePath, string(*versioned.TemplatePath))
			require.Equal(t, r.TemplateKind, string(*versioned.TemplateKind))
			if r.TemplateKind == types.TemplateKindTerraform {
				require.Equal(t, r.TemplateVersion, string(*versioned.TemplateVersion))
			}
			require.Equal(t, r.Parameters, versioned.Parameters)
		})
	}
}

func TestEnvironmentRecipePropertiesConvertDataModelToVersioned_EmptyTemplateKind(t *testing.T) {
	filename := "environmentrecipepropertiesdatamodel-missingtemplatekind.json"
	t.Run(filename, func(t *testing.T) {
		rawPayload := testutil.ReadFixture(filename)
		r := &datamodel.EnvironmentRecipeProperties{}
		err := json.Unmarshal(rawPayload, r)
		require.NoError(t, err)

		// act
		versioned := &RecipeMetadataProperties{}
		err = versioned.ConvertFrom(r)
		// assert
		require.NoError(t, err)
		require.Equal(t, r.TemplatePath, string(*versioned.TemplatePath))
		require.Equal(t, r.TemplateKind, string(*versioned.TemplateKind))
		if r.TemplateKind == types.TemplateKindTerraform {
			require.Equal(t, r.TemplateVersion, string(*versioned.TemplateVersion))
		}
		require.Equal(t, r.Parameters, versioned.Parameters)
	})
}

func TestRecipeConvertVersionedToDataModel(t *testing.T) {
	t.Run("Convert to Data Model", func(t *testing.T) {
		filename := "reciperesource.json"
		expected := &datamodel.Recipe{
			LinkType: linkrp.MongoDatabasesResourceType,
			Name:     "mongo-azure",
		}
		rawPayload := testutil.ReadFixture(filename)
		r := &Recipe{}
		err := json.Unmarshal(rawPayload, r)
		require.NoError(t, err)
		// act
		dm, err := r.ConvertTo()
		require.NoError(t, err)
		ct := dm.(*datamodel.Recipe)
		require.Equal(t, expected, ct)
	})
}
