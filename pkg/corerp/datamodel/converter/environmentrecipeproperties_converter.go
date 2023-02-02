// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// EnvironmentRecipePropertiesDataModelToVersioned converts version agnostic environment recipe properties datamodel to versioned model.
func EnvironmentRecipePropertiesDataModelToVersioned(model *datamodel.EnvironmentRecipeProperties, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.EnvironmentRecipeProperties{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// EnvironmentRecipePropertiesDataModelFromVersioned converts versioned environment recipe properties model to datamodel.
func EnvironmentRecipePropertiesDataModelFromVersioned(content []byte, version string) (*datamodel.EnvironmentRecipeProperties, error) {
	switch version {

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
