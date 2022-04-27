// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func loadTestData(testfile string) []byte {
	d, err := ioutil.ReadFile(testfile)
	if err != nil {
		return nil
	}
	return d
}

// NOTENOTE: this test is to validate the type conversion between versioned model and data model.
// Converted content must be tested in ConvertFrom and ConvertTo tests in api models under /pkg/api/[api-version].

func TestEnvironmentDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  interface{}
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/environmentresourcedatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.EnvironmentResource{},
			nil,
		},
		// TODO: add new conversion tests.
		{
			"",
			"unsupported",
			nil,
			datamodel.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := loadTestData(tc.dataModelFile)
			dm := &datamodel.Environment{}
			_ = json.Unmarshal(c, dm)
			am, err := EnvironmentDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}

func TestEnvironmentDataModelFromVersioned(t *testing.T) {
	testset := []struct {
		versionedModelFile string
		apiVersion         string
		err                error
	}{
		{
			"../../api/v20220315privatepreview/testdata/environmentresource.json",
			"2022-03-15-privatepreview",
			nil,
		},
		// TODO: add new conversion tests.
		{
			"",
			"unsupported",
			datamodel.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := loadTestData(tc.versionedModelFile)
			dm, err := EnvironmentDataModelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiVersion, dm.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}
