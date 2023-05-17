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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/daprrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/daprrp/datamodel"
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
			"../../api/v20220315privatepreview/testdata/statestoresqlserverresourcedatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.DaprStateStoreResource{},
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/statestoreazuretablestorageresourcedatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.DaprStateStoreResource{},
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/statestogenericreresourcedatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.DaprStateStoreResource{},
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
			am, err := StateStoreDataModelToVersioned(dm, tc.apiVersion)
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
			"../../api/v20220315privatepreview/testdata/statestoresqlserverresource.json",
			"2022-03-15-privatepreview",
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/statestoreazuretablestorageresource.json",
			"2022-03-15-privatepreview",
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/statestogenericreresource.json",
			"2022-03-15-privatepreview",
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/statestoreresource-invalid.json",
			"2022-03-15-privatepreview",
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
			dm, err := StateStoreDataModelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiVersion, dm.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}
