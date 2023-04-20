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

// VolumeResourceModelToVersioned converts version agnostic Volume datamodel to versioned model.
func VolumeResourceModelToVersioned(model *datamodel.VolumeResource, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.VolumeResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// VolumeResourceModelFromVersioned converts versioned Volume model to datamodel.
func VolumeResourceModelFromVersioned(content []byte, version string) (*datamodel.VolumeResource, error) {
	switch version {
	case v20230415preview.Version:
		am := &v20230415preview.VolumeResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.VolumeResource), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
