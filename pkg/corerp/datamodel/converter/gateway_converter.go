// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// GatewayDataModelToVersioned converts version agnostic Gateway datamodel to versioned model.
func GatewayDataModelToVersioned(model *datamodel.Gateway, version string) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.GatewayResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// GatewayDataModelFromVersioned converts versioned Gateway model to datamodel.
func GatewayDataModelFromVersioned(content interface{}, version string) (*datamodel.Gateway, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.GatewayResource{}
		if err := store.DecodeMap(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.Gateway), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
