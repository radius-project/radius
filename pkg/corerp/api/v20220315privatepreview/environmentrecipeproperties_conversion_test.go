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
	filename := "environmentrecipeproperties.json"
	expected := &datamodel.EnvironmentRecipeProperties{
		LinkType:     linkrp.MongoDatabasesResourceType,
		TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/mongodatabases",
		Parameters: map[string]any{
			"location": map[string]any{
				"defaultValue": "[resourceGroup().location]",
				"type":         "string",
			},
			"throughput": map[string]any{
				"defaultValue": (float64(200)),
				"maxValue":     (float64(400)),
			},
		},
	}

	t.Run(filename, func(t *testing.T) {
		rawPayload := testutil.ReadFixture(filename)
		r := &EnvironmentRecipeProperties{}
		err := json.Unmarshal(rawPayload, r)
		require.NoError(t, err)

		// act
		dm, err := r.ConvertTo()

		require.NoError(t, err)
		ct := dm.(*datamodel.EnvironmentRecipeProperties)
		require.Equal(t, expected, ct)
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
		expectedOutput := map[string]any{
			"location": map[string]any{
				"defaultValue": "[resourceGroup().location]",
				"type":         "string",
			},
			"throughput": map[string]any{
				"defaultValue": (float64(200)),
				"maxValue":     (float64(400)),
			},
		}
		// assert
		require.NoError(t, err)
		require.Equal(t, "Applications.Link/mongoDatabases", string(*versioned.LinkType))
		require.Equal(t, "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb", string(*versioned.TemplatePath))
		require.Equal(t, expectedOutput, versioned.Parameters)
	})
}
