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

// ContainerDataModelToVersioned converts version agnostic Container datamodel to versioned model.
func ContainerDataModelToVersioned(model *datamodel.ContainerResource, version string) (api.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.ContainerResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}

// ContainerDataModelFromVersioned converts versioned Container model to datamodel.
func ContainerDataModelFromVersioned(content []byte, version string) (*datamodel.ContainerResource, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.ContainerResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.ContainerResource), err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}
