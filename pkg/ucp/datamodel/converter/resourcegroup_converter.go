// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v20230415preview "github.com/project-radius/radius/pkg/ucp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

// ResourceGroupDataModelToVersioned converts version agnostic environment datamodel to versioned model.
func ResourceGroupDataModelToVersioned(model *datamodel.ResourceGroup, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.ResourceGroupResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// ResourceGroupDataModelFromVersioned converts versioned environment model to datamodel.
func ResourceGroupDataModelFromVersioned(content []byte, version string) (*datamodel.ResourceGroup, error) {
	switch version {
	case v20230415preview.Version:
		vm := &v20230415preview.ResourceGroupResource{}
		if err := json.Unmarshal(content, vm); err != nil {
			return nil, err
		}
		dm, err := vm.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.ResourceGroup), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
