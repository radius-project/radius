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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	v20220315privatepreview "github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// EnvironmentDataModelToVersioned converts version agnostic environment datamodel to versioned model.
func EnvironmentDataModelToVersioned(model *datamodel.Environment, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.EnvironmentResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// EnvironmentDataModelFromVersioned converts versioned environment model to datamodel.
func EnvironmentDataModelFromVersioned(content []byte, version string) (*datamodel.Environment, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.EnvironmentResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.Environment), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
