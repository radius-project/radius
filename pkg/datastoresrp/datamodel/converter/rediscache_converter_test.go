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
	"github.com/project-radius/radius/pkg/datastoresrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	"github.com/stretchr/testify/require"
)

// Validates type conversion between versioned client side data model and RP data model.
func TestRedisCacheDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/rediscacheresourcedatamodel_manual.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.RedisCacheResource{},
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/rediscacheresourcedatamodel_manual.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c, err := v20220315privatepreview.LoadTestData(tc.dataModelFile)
			require.NoError(t, err)
			dm := &datamodel.RedisCache{}
			_ = json.Unmarshal(c, dm)
			am, err := RedisCacheDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}

func TestRedisCacheDataModelFromVersioned(t *testing.T) {
	testset := []struct {
		versionedModelFile string
		apiVersion         string
		err                error
	}{
		{
			"../../api/v20220315privatepreview/testdata/rediscacheresource_manual.json",
			"2022-03-15-privatepreview",
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/rediscacheresource-invalidinput.json",
			"2022-03-15-privatepreview",
			errors.New("json: cannot unmarshal number into Go struct field RedisCacheProperties.properties.host of type string"),
		},
		{
			"../../api/v20220315privatepreview/testdata/rediscacheresource-invalid2.json",
			"2022-03-15-privatepreview",
			&v1.ErrClientRP{Code: "BadRequest", Message: "multiple errors were found:\n\thost must be specified when resourceProvisioning is set to manual\n\tport must be specified when resourceProvisioning is set to manual"},
		},
		{
			"../../api/v20220315privatepreview/testdata/rediscacheresource-invalid2.json",
			"unsupported",
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c, err := v20220315privatepreview.LoadTestData(tc.versionedModelFile)
			require.NoError(t, err)
			dm, err := RedisCacheDataModelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiVersion, dm.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}

func TestRedisCacheSecretsDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20220315privatepreview/testdata/rediscachesecretsdatamodel.json",
			"2022-03-15-privatepreview",
			&v20220315privatepreview.RedisCacheSecrets{},
			nil,
		},
		{
			"../../api/v20220315privatepreview/testdata/rediscachesecretsdatamodel.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c, err := v20220315privatepreview.LoadTestData(tc.dataModelFile)
			require.NoError(t, err)
			dm := &datamodel.RedisCacheSecrets{}
			_ = json.Unmarshal(c, dm)
			am, err := RedisCacheSecretsDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}
