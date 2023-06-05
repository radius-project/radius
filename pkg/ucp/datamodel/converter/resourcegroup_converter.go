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
	v20220901privatepreview "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
)

// ResourceGroupDataModelToVersioned converts version agnostic environment datamodel to versioned model.
//
// # Function Explanation
// 
//	ResourceGroupDataModelToVersioned takes in a ResourceGroup model and a version string and returns a 
//	VersionedModelInterface object. It checks the version string and converts the model to the corresponding versioned 
//	model, returning an error if the version is unsupported.
func ResourceGroupDataModelToVersioned(model *datamodel.ResourceGroup, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220901privatepreview.Version:
		versioned := &v20220901privatepreview.ResourceGroupResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// ResourceGroupDataModelFromVersioned converts versioned environment model to datamodel.
//
// # Function Explanation
// 
//	ResourceGroupDataModelFromVersioned takes in a byte array and a version string and returns a ResourceGroup object or an 
//	error. It uses a switch statement to determine which version of the ResourceGroup object to create, and then uses the 
//	ConvertTo method to convert the versioned object to the ResourceGroup object. If an error occurs during the conversion, 
//	it is returned to the caller.
func ResourceGroupDataModelFromVersioned(content []byte, version string) (*datamodel.ResourceGroup, error) {
	switch version {
	case v20220901privatepreview.Version:
		vm := &v20220901privatepreview.ResourceGroupResource{}
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
