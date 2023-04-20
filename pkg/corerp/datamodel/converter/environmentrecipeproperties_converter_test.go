// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v20230415preview "github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
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
			"../../api/v20230415preview/testdata/environmentrecipepropertiesdatamodel.json",
			"2023-04-15-preview",
			&v20230415preview.EnvironmentRecipeProperties{},
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
