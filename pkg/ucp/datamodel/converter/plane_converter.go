// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

// PlaneDataModelToVersioned converts version agnostic plane datamodel to versioned model.
func PlaneDataModelToVersioned(model *datamodel.Plane, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.PlaneResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// PlaneDataModelFromVersioned converts versioned plane model to datamodel.
func PlaneDataModelFromVersioned(content []byte, version string) (*datamodel.Plane, error) {
	switch version {
	case v20230415preview.Version:
		vm := &v20230415preview.PlaneResource{}
		if err := json.Unmarshal(content, vm); err != nil {
			return nil, err
		}
		dm, err := vm.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.Plane), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
