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

// AzureCredentialDataModelToVersioned converts version agnostic Azure credential datamodel to versioned model.
func AzureCredentialDataModelToVersioned(model *datamodel.AzureCredential, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.AzureCredentialResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// AzureCredentialDataModelFromVersioned converts versioned Azure credential model to datamodel.
func AzureCredentialDataModelFromVersioned(content []byte, version string) (*datamodel.AzureCredential, error) {
	switch version {
	case v20230415preview.Version:
		vm := &v20230415preview.AzureCredentialResource{}
		if err := json.Unmarshal(content, vm); err != nil {
			return nil, err
		}
		dm, err := vm.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.AzureCredential), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
