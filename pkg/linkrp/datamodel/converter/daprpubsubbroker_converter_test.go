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
func TestDaprPubSubBrokerDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20230415preview/testdata/daprpubsubbrokerazureresourcedatamodel.json",
			"2023-04-15-preview",
			&v20230415preview.DaprPubSubBrokerResource{},
			nil,
		},
		{
			"../../api/v20230415preview/testdata/daprpubsubbrokergenericresourcedatamodel.json",
			"2023-04-15-preview",
			&v20230415preview.DaprPubSubBrokerResource{},
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
			dm := &datamodel.DaprPubSubBroker{}
			_ = json.Unmarshal(c, dm)
			am, err := DaprPubSubBrokerDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}

func TestDaprPubSubBrokerDataModelFromVersioned(t *testing.T) {
	testset := []struct {
		versionedModelFile string
		apiVersion         string
		err                error
	}{
		{
			"../../api/v20230415preview/testdata/daprpubsubbrokerazureresource.json",
			"2023-04-15-preview",
			nil,
		},
		{
			"../../api/v20230415preview/testdata/daprpubsubbrokergenericresource.json",
			"2023-04-15-preview",
			nil,
		},
		{
			"../../api/v20230415preview/testdata/daprpubsubbrokerresource-invalid.json",
			"2023-04-15-preview",
			errors.New("json: cannot unmarshal number into Go struct field DaprPubSubBrokerProperties.properties.resource of type string"),
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
			dm, err := DaprPubSubBrokerDataModelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiVersion, dm.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}
