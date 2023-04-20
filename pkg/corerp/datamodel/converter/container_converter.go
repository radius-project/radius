// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v20230415preview "github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// ContainerDataModelToVersioned converts version agnostic Container datamodel to versioned model.
func ContainerDataModelToVersioned(model *datamodel.ContainerResource, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.ContainerResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// ContainerDataModelFromVersioned converts versioned Container model to datamodel.
func ContainerDataModelFromVersioned(content []byte, version string) (*datamodel.ContainerResource, error) {
	switch version {
	case v20230415preview.Version:
		am := &v20230415preview.ContainerResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.ContainerResource), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
