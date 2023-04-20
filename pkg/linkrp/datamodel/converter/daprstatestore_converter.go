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

// DaprStateStoreDataModelFromVersioned converts version agnostic DaprStateStore datamodel to versioned model.
func DaprStateStoreDataModelToVersioned(model *datamodel.DaprStateStore, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.DaprStateStoreResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// DaprStateStoreDataModelToVersioned converts versioned DaprStateStore model to datamodel.
func DaprStateStoreDataModelFromVersioned(content []byte, version string) (*datamodel.DaprStateStore, error) {
	switch version {
	case v20230415preview.Version:
		am := &v20230415preview.DaprStateStoreResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.DaprStateStore), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
