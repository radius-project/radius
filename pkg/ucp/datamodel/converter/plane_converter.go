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

// PlaneDataModelToVersioned converts version agnostic plane datamodel to versioned model.
//
// # Function Explanation
// 
//	PlaneDataModelToVersioned takes in a Plane data model and a version string, and returns a VersionedModelInterface object
//	 for the specified version. If the version is not supported, it returns an error.
func PlaneDataModelToVersioned(model *datamodel.Plane, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220901privatepreview.Version:
		versioned := &v20220901privatepreview.PlaneResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// PlaneDataModelFromVersioned converts versioned plane model to datamodel.
//
// # Function Explanation
// 
//	PlaneDataModelFromVersioned takes in a byte array and a version string and returns a Plane data model. It uses a switch 
//	statement to determine which version of the data model to use, and then converts the byte array to the corresponding 
//	versioned data model before converting it to the Plane data model. If an unsupported version is provided, it returns an 
//	error.
func PlaneDataModelFromVersioned(content []byte, version string) (*datamodel.Plane, error) {
	switch version {
	case v20220901privatepreview.Version:
		vm := &v20220901privatepreview.PlaneResource{}
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
