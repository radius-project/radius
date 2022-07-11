// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// EnvironmentDataModelToVersioned converts version agnostic environment datamodel to versioned model.
func EnvironmentDataModelToVersioned(model *datamodel.Environment, version string) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.EnvironmentResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// EnvironmentDataModelFromVersioned converts versioned environment model to datamodel.
func EnvironmentDataModelFromVersioned(content []byte, version string) (*datamodel.Environment, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.EnvironmentResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.Environment), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
