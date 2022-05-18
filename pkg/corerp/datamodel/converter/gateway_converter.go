// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// GatewayDataModelToVersioned converts version agnostic Gateway datamodel to versioned model.
func GatewayDataModelToVersioned(model *datamodel.Gateway, version string) (api.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.GatewayResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}

// GatewayDataModelFromVersioned converts versioned Gateway model to datamodel.
func GatewayDataModelFromVersioned(content []byte, version string) (*datamodel.Gateway, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.GatewayResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.Gateway), err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}
