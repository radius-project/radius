// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/stretchr/testify/require"
)

func loadTestData(testfile string) []byte {
	d, err := os.ReadFile(testfile)
	if err != nil {
		return nil
	}
	return d
}

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
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := loadTestData(tc.dataModelFile)
			dm := &datamodel.MongoDatabase{}
			_ = json.Unmarshal(c, dm)
			am, err := MongoDatabaseDataModelToVersioned(dm, tc.apiVersion, true)
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
			errors.New("json: cannot unmarshal number into Go struct field MongoDatabaseProperties.properties.resource of type string"),
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

func TestMongoDatabaseResponseDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  interface{}
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/mongodatabaseresponseresourcedatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.MongoDatabaseResponseResource{},
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
			dm := &datamodel.MongoDatabaseResponse{}
			_ = json.Unmarshal(c, dm)
			am, err := MongoDatabaseDataModelToVersioned(dm, tc.apiVersion, false)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}
func TestMongoDatabaseSecretsDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  interface{}
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/mongodatabasesecretsdatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.MongoDatabaseSecrets{},
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
			dm := &datamodel.MongoDatabaseSecrets{}
			_ = json.Unmarshal(c, dm)
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
