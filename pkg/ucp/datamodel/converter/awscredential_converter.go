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
func AWSCredentialDataModelToVersioned(model *datamodel.AWSCredential, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220901privatepreview.Version:
		versioned := &v20220901privatepreview.AWSCredentialResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// CredentialDataModelFromVersioned converts versioned credential model to datamodel.
func AWSCredentialDataModelFromVersioned(content []byte, version string) (*datamodel.AWSCredential, error) {
	switch version {
	case v20220901privatepreview.Version:
		vm := &v20220901privatepreview.AWSCredentialResource{}
		if err := json.Unmarshal(content, vm); err != nil {
			return nil, err
		}
		dm, err := vm.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.AWSCredential), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
