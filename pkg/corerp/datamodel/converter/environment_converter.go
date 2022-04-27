// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/corerp/api"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// EnvironmentDataModelToVersioned converts version agnostic environment datamodel to versioned model.
func EnvironmentDataModelToVersioned(model *datamodel.Environment, version string) (api.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.EnvironmentResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, datamodel.ErrUnsupportedAPIVersion
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
		return dm.(*datamodel.Environment), err

	default:
		return nil, datamodel.ErrUnsupportedAPIVersion
	}
}
