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

// ApplicationDataModelToVersioned converts version agnostic application datamodel to versioned model.
func ApplicationDataModelToVersioned(model *datamodel.Application, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.ApplicationResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// ApplicationDataModelFromVersioned converts versioned application model to datamodel.
func ApplicationDataModelFromVersioned(content []byte, version string) (*datamodel.Application, error) {
	switch version {
	case v20230415preview.Version:
		am := &v20230415preview.ApplicationResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.Application), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
