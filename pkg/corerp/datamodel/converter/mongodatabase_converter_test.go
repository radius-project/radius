// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"
	"errors"
	"testing"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

// Validates type conversion between versioned client side data model and RP data model.
func TestMongoDatabaseDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  interface{}
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/mongodatabaseresourcedatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.MongoDatabaseResource{},
			nil,
		},
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
			dm := &datamodel.MongoDatabase{}
			_ = json.Unmarshal(c, dm)
			am, err := MongoDatabaseDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}

func TestMongoDatabaseDataModelFromVersioned(t *testing.T) {
	testset := []struct {
		versionedModelFile string
		apiVersion         string
		err                error
	}{
		{
			"../../api/v20220315privatepreview/testdata/mongodatabaseresource.json",
			"2022-03-15-privatepreview",
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/mongodatabaseresource-invalid.json",
			"2022-03-15-privatepreview",
			errors.New("json: cannot unmarshal number into Go struct field FromResource.properties.fromResource.source of type string"),
		},
		{
			"",
			"unsupported",
			datamodel.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := loadTestData(tc.versionedModelFile)
			dm, err := MongoDatabaseDataModelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiVersion, dm.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}
