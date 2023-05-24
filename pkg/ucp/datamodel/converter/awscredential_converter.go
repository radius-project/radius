/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

// AWSCredentialDataModelToVersioned converts version agnostic AWS credential datamodel to versioned model.
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

// AWSCredentialDataModelFromVersioned converts AWS versioned credential model to datamodel.
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
