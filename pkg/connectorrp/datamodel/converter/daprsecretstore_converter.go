// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
)

// DaprSecretStoreDataModelFromVersioned converts version agnostic DaprSecretStore datamodel to versioned model.
func DaprSecretStoreDataModelToVersioned(model *datamodel.DaprSecretStore, version string) (api.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.DaprSecretStoreResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}

// DaprSecretStoreDataModelToVersioned converts versioned DaprSecretStore model to datamodel.
func DaprSecretStoreDataModelFromVersioned(content []byte, version string) (*datamodel.DaprSecretStore, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.DaprSecretStoreResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.DaprSecretStore), err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}
