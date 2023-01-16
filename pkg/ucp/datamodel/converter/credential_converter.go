// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

// CredentialDataModelToVersioned converts version agnostic credential datamodel to versioned model.
func CredentialDataModelToVersioned(model *datamodel.Credential, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220901privatepreview.Version:
		versioned := &v20220901privatepreview.CredentialResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// CredentialDataModelFromVersioned converts versioned credential model to datamodel.
func CredentialDataModelFromVersioned(content []byte, version string) (*datamodel.Credential, error) {
	switch version {
	case v20220901privatepreview.Version:
		vm := &v20220901privatepreview.CredentialResource{}
		if err := json.Unmarshal(content, vm); err != nil {
			return nil, err
		}
		dm, err := vm.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.Credential), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
