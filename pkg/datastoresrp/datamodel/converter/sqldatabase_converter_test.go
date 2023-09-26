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
	"github.com/radius-project/radius/pkg/datastoresrp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	"github.com/radius-project/radius/test/testutil"
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
			"../../api/v20231001preview/testdata/sqldatabase_manual_resourcedatamodel.json",
			"2023-10-01-preview",
			&v20231001preview.SQLDatabaseResource{},
			nil,
		},
		{
			"../../api/v20231001preview/testdata/sqldatabase_manual_resourcedatamodel.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.dataModelFile)
			dm := &datamodel.SqlDatabase{}
			err := json.Unmarshal(c, dm)
			require.NoError(t, err)
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
			"../../api/v20231001preview/testdata/sqldatabase_manual_resource.json",
			"2023-10-01-preview",
			nil,
		},
		{
			"../../api/v20231001preview/testdata/sqldatabase_recipe_resource.json",
			"2023-10-01-preview",
			nil,
		},
		{
			"../../api/v20231001preview/testdata/sqldatabaseresource-invalid.json",
			"2023-10-01-preview",
			errors.New("json: cannot unmarshal number into Go struct field SqlDatabaseProperties.properties.database of type string"),
		},
		{
			"../../api/v20231001preview/testdata/sqldatabase_invalid_properties_resource.json",
			"2023-10-01-preview",
			&v1.ErrClientRP{Code: v1.CodeInvalid, Message: "multiple errors were found:\n\tserver must be specified when resourceProvisioning is set to manual\n\tport must be specified when resourceProvisioning is set to manual\n\tdatabase must be specified when resourceProvisioning is set to manual"},
		},
		{
			"../../api/v20231001preview/testdata/sqldatabase_invalid_properties_resource.json",
			"unsupported",
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.versionedModelFile)
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
			"../../api/v20231001preview/testdata/sqldatabase_secrets_datamodel.json",
			"2023-10-01-preview",
			&v20231001preview.SQLDatabaseSecrets{},
			nil,
		},
		{
			"../../api/v20231001preview/testdata/sqldatabase_recipe_resourcedatamodel.json",
			"2023-10-01-preview",
			&v20231001preview.SQLDatabaseSecrets{},
			nil,
		},
		{
			"../../api/v20231001preview/testdata/sqldatabase_recipe_resourcedatamodel.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.dataModelFile)
			dm := &datamodel.SqlDatabaseSecrets{}
			err := json.Unmarshal(c, dm)
			require.NoError(t, err)
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
