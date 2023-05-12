// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/daprrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/daprrp/datamodel"
)

// SecretStoreDataModelFromVersioned converts version agnostic Dapr SecretStore datamodel to versioned model.
func SecretStoreDataModelToVersioned(model *datamodel.DaprSecretStore, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.DaprSecretStoreResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// SecretStoreDataModelToVersioned converts versioned DaprSecretStore model to datamodel.
func SecretStoreDataModelFromVersioned(content []byte, version string) (*datamodel.DaprSecretStore, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.DaprSecretStoreResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.DaprSecretStore), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
