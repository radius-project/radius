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
func TestMongoDatabaseDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20231001preview/testdata/mongodatabaseresourcedatamodel.json",
			"2023-10-01-preview",
			&v20231001preview.MongoDatabaseResource{},
			nil,
		},
		{
			"../../api/v20231001preview/testdata/mongodatabaseresource-missinginputs.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.dataModelFile)
			dm := &datamodel.MongoDatabase{}
			err := json.Unmarshal(c, dm)
			require.NoError(t, err)
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
			"../../api/v20231001preview/testdata/mongodatabaseresource.json",
			"2023-10-01-preview",
			nil,
		},
		{
			"../../api/v20231001preview/testdata/mongodatabaseresource-invalid.json",
			"2023-10-01-preview",
			errors.New("json: cannot unmarshal number into Go struct field MongoDatabaseProperties.properties.resource of type string"),
		},
		{
			"../../api/v20231001preview/testdata/mongodatabaseresource-missinginputs.json",
			"2023-10-01-preview",
			&v1.ErrClientRP{Code: "BadRequest", Message: "multiple errors were found:\n\thost must be specified when resourceProvisioning is set to manual\n\tport must be specified when resourceProvisioning is set to manual\n\tdatabase must be specified when resourceProvisioning is set to manual"},
		},
		{
			"../../api/v20231001preview/testdata/mongodatabaseresource-missinginputs.json",
			"unsupported",
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.versionedModelFile)
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

func TestMongoDatabaseSecretsDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20231001preview/testdata/mongodatabasesecretsdatamodel.json",
			"2023-10-01-preview",
			&v20231001preview.MongoDatabaseSecrets{},
			nil,
		},
		{
			"../../api/v20231001preview/testdata/mongodatabasesecretsdatamodel.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.dataModelFile)
			dm := &datamodel.MongoDatabaseSecrets{}
			err := json.Unmarshal(c, dm)
			require.NoError(t, err)
			am, err := MongoDatabaseSecretsDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}
