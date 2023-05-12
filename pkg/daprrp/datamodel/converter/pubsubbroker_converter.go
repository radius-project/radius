// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"
	"os"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/daprrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/daprrp/datamodel"
)

// PubSubBrokerDataModelFromVersioned converts version agnostic DaprPubSubBroker datamodel to versioned model.
func PubSubBrokerDataModelToVersioned(model *datamodel.DaprPubSubBroker, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.DaprPubSubBrokerResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// PubSubBrokerDataModelToVersioned converts versioned DaprPubSubBroker model to datamodel.
func PubSubBrokerDataModelFromVersioned(content []byte, version string) (*datamodel.DaprPubSubBroker, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.DaprPubSubBrokerResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.DaprPubSubBroker), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

func loadTestData(testfile string) []byte {
	d, err := os.ReadFile(testfile)
	if err != nil {
		return nil
	}
	return d
}
