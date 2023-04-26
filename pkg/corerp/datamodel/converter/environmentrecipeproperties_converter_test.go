// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

// NOTENOTE: this test is to validate the type conversion between versioned model and data model.
// Converted content must be tested in ConvertFrom and ConvertTo tests in api models under /pkg/api/[api-version].

func TestEnvironmentRecipePropertiesDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/environmentrecipepropertiesdatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.EnvironmentRecipeProperties{},
			nil,
		},
		// TODO: add new conversion tests.
		{
			"",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := loadTestData(tc.dataModelFile)
			dm := &datamodel.EnvironmentRecipeProperties{}
			_ = json.Unmarshal(c, dm)
			am, err := EnvironmentRecipePropertiesDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}

func TestRecipeNameLinkTypeDatamodelFromVersioned(t *testing.T) {
	testset := []struct {
		versionedModelFile string
		apiVersion         string
		err                error
	}{
		{
			"../../api/v20220315privatepreview/testdata/reciperesource.json",
			"2022-03-15-privatepreview",
			nil,
		},
		// TODO: add new conversion tests.
		{
			"",
			"unsupported",
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := loadTestData(tc.versionedModelFile)
			_, err := RecipeNameLinkTypeDatamodelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
