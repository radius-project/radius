// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
)

// DaprPubSubBrokerDataModelFromVersioned converts version agnostic DaprPubSubBroker datamodel to versioned model.
func DaprPubSubBrokerDataModelToVersioned(model *datamodel.DaprPubSubBroker, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.DaprPubSubBrokerResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// DaprPubSubBrokerDataModelToVersioned converts versioned DaprPubSubBroker model to datamodel.
func DaprPubSubBrokerDataModelFromVersioned(content []byte, version string) (*datamodel.DaprPubSubBroker, error) {
	switch version {
	case v20230415preview.Version:
		am := &v20230415preview.DaprPubSubBrokerResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.DaprPubSubBroker), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
