// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func TestEnvironmentRecipePropertiesConvertVersionedToDataModel(t *testing.T) {
	t.Run("Convert to Data Model", func(t *testing.T) {
		r := &EnvironmentRecipeProperties{}
		// act
		_, err := r.ConvertTo()

		require.ErrorContains(t, err, "converting Environment Recipe Properties to a version-agnostic object is not supported")
	})
}

func TestEnvironmentRecipePropertiesConvertDataModelToVersioned(t *testing.T) {
	filename := "environmentrecipepropertiesdatamodel.json"
	t.Run(filename, func(t *testing.T) {
		rawPayload := testutil.ReadFixture(filename)
		r := &datamodel.EnvironmentRecipeProperties{}
		err := json.Unmarshal(rawPayload, r)
		require.NoError(t, err)

		// act
		versioned := &EnvironmentRecipeProperties{}
		err = versioned.ConvertFrom(r)
		// assert
		require.NoError(t, err)
		require.Equal(t, r.TemplatePath, string(*versioned.TemplatePath))
		require.Equal(t, r.TemplateKind, string(*versioned.TemplateKind))
		require.Equal(t, r.Parameters, versioned.Parameters)
	})
}

func TestEnvironmentRecipePropertiesConvertDataModelToVersioned_EmptyTemplateKind(t *testing.T) {
	filename := "environmentrecipepropertiesdatamodel-missingtemplatekind.json"
	t.Run(filename, func(t *testing.T) {
		rawPayload := testutil.ReadFixture(filename)
		r := &datamodel.EnvironmentRecipeProperties{}
		err := json.Unmarshal(rawPayload, r)
		require.NoError(t, err)

		// act
		versioned := &EnvironmentRecipeProperties{}
		err = versioned.ConvertFrom(r)
		// assert
		require.NoError(t, err)
		require.Equal(t, r.TemplatePath, string(*versioned.TemplatePath))
		require.Equal(t, r.TemplateKind, string(*versioned.TemplateKind))
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
