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

package converter

import (
	"encoding/json"
	"errors"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/datastoresrp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	linkrp_util "github.com/radius-project/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/stretchr/testify/require"
)

// Validates type conversion between versioned client side data model and RP data model.
func TestSqlDatabaseDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/sqldatabase_manual_resourcedatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.SQLDatabaseResource{},
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/sqldatabase_manual_resourcedatamodel.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c, err := linkrp_util.LoadTestData(tc.dataModelFile)
			require.NoError(t, err)
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
			"../../api/v20220315privatepreview/testdata/sqldatabase_manual_resource.json",
			"2022-03-15-privatepreview",
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/sqldatabase_recipe_resource.json",
			"2022-03-15-privatepreview",
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/sqldatabaseresource-invalid.json",
			"2022-03-15-privatepreview",
			errors.New("json: cannot unmarshal number into Go struct field SqlDatabaseProperties.properties.database of type string"),
		},
		{
			"../../api/v20220315privatepreview/testdata/sqldatabase_invalid_properties_resource.json",
			"2022-03-15-privatepreview",
			&v1.ErrClientRP{Code: v1.CodeInvalid, Message: "multiple errors were found:\n\tserver must be specified when resourceProvisioning is set to manual\n\tport must be specified when resourceProvisioning is set to manual\n\tdatabase must be specified when resourceProvisioning is set to manual"},
		},
		{
			"../../api/v20220315privatepreview/testdata/sqldatabase_invalid_properties_resource.json",
			"unsupported",
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c, err := linkrp_util.LoadTestData(tc.versionedModelFile)
			require.NoError(t, err)
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

func TestSqlDatabaseSecretsDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/sqldatabase_secrets_datamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.SQLDatabaseSecrets{},
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/sqldatabase_recipe_resourcedatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.SQLDatabaseSecrets{},
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/sqldatabase_recipe_resourcedatamodel.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c, err := linkrp_util.LoadTestData(tc.dataModelFile)
			require.NoError(t, err)
			dm := &datamodel.SqlDatabaseSecrets{}
			_ = json.Unmarshal(c, dm)
			am, err := SqlDatabaseSecretsDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}
