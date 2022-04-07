// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"
	"errors"

	"github.com/project-radius/radius/pkg/corerp/api"
	v20220315 "github.com/project-radius/radius/pkg/corerp/api/v20220315"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

var ErrUnsupportedAPIVersion = errors.New("unsupported api-version")

// EnvironmentDataModelFromVersioned converts version agnostic environment datamodel to versioned model.
func EnvironmentDataModelToVersioned(model *datamodel.Environment, version string) (api.VersionedModelInterface, error) {
	switch version {
	case v20220315.Version:
		versioned := &v20220315.Environment{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, ErrUnsupportedAPIVersion
	}
}

// EnvironmentDataModelToVersioned converts versioned environment model to datamodel.
func EnvironmentDataModelFromVersioned(content []byte, version string) (*datamodel.Environment, error) {
	switch version {
	case v20220315.Version:
		am := &v20220315.Environment{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.Environment), err

	default:
		return nil, ErrUnsupportedAPIVersion
	}
}
