// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
)

// ExtenderDataModelFromVersioned converts version agnostic Extender datamodel to versioned model.
func ExtenderDataModelToVersioned(model *datamodel.Extender, version string) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.ExtenderResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// ExtenderDataModelToVersioned converts versioned Extender model to datamodel.
func ExtenderDataModelFromVersioned(content []byte, version string) (*datamodel.Extender, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.ExtenderResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.Extender), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
