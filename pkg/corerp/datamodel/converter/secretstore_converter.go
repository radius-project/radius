// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// SecretStoreModelToVersioned converts version agnostic SecretStore datamodel to versioned model.
func SecretStoreModelToVersioned(model *datamodel.VolumeResource, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.SecretStoreResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// SecretStoreModelFromVersioned converts versioned SecretStore model to datamodel.
func SecretStoreModelFromVersioned(content []byte, version string) (*datamodel.SecretStore, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.SecretStoreResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.SecretStore), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
