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
	"github.com/project-radius/radius/pkg/linkrp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/stretchr/testify/require"
)

// Validates type conversion between versioned client side data model and RP data model.
func TestDaprStateStoreDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20230415preview/testdata/daprstatestoresqlserverresourcedatamodel.json",
			"2023-04-15-preview",
			&v20230415preview.DaprStateStoreResource{},
			nil,
		},
		{
			"../../api/v20230415preview/testdata/daprstatestoreazuretablestorageresourcedatamodel.json",
			"2023-04-15-preview",
			&v20230415preview.DaprStateStoreResource{},
			nil,
		},
		{
			"../../api/v20230415preview/testdata/daprstatestogenericreresourcedatamodel.json",
			"2023-04-15-preview",
			&v20230415preview.DaprStateStoreResource{},
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
			dm := &datamodel.DaprStateStore{}
			_ = json.Unmarshal(c, dm)
			am, err := DaprStateStoreDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}

func TestDaprStateStoreDataModelFromVersioned(t *testing.T) {
	testset := []struct {
		versionedModelFile string
		apiVersion         string
		err                error
	}{
		{
			"../../api/v20230415preview/testdata/daprstatestoresqlserverresource.json",
			"2023-04-15-preview",
			nil,
		},
		{
			"../../api/v20230415preview/testdata/daprstatestoreazuretablestorageresource.json",
			"2023-04-15-preview",
			nil,
		},
		{
			"../../api/v20230415preview/testdata/daprstatestogenericreresource.json",
			"2023-04-15-preview",
			nil,
		},
		{
			"../../api/v20230415preview/testdata/daprstatestoreresource-invalid.json",
			"2023-04-15-preview",
			errors.New("json: cannot unmarshal number into Go struct field DaprStateStoreProperties.properties.resource of type string"),
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
			dm, err := DaprStateStoreDataModelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiVersion, dm.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}
