// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"
	"errors"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/stretchr/testify/require"
)

// Validates type conversion between versioned client side data model and RP data model.
func TestSqlDatabaseDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  interface{}
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/sqldatabaseresourcedatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.SQLDatabaseResource{},
			nil,
		},
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
			dm := &datamodel.SqlDatabase{}
			_ = json.Unmarshal(c, dm)
			am, err := SqlDatabaseDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}

func TestSqlDatabaseDataModelFromVersioned(t *testing.T) {
	testset := []struct {
		versionedModelFile string
		apiVersion         string
		err                error
	}{
		{
			"../../api/v20220315privatepreview/testdata/sqldatabaseresource.json",
			"2022-03-15-privatepreview",
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/sqldatabaseresource-invalid.json",
			"2022-03-15-privatepreview",
			errors.New("json: cannot unmarshal number into Go struct field SqlDatabaseProperties.properties.resource of type string"),
		},
		{
			"",
			"unsupported",
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := loadTestData(tc.versionedModelFile)
			dm, err := SqlDatabaseDataModelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiVersion, dm.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}
