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
	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
)

// ExtenderDataModelToVersioned converts a datamodel.Extender to a versioned model interface based on the given version
// string, returning an error if the conversion fails.
func ExtenderDataModelToVersioned(model *datamodel.Extender, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.ExtenderResource{}
		err := versioned.ConvertFrom(model)
		if err != nil {
			return nil, err
		}

		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// ExtenderDataModelFromVersioned unmarshals a JSON byte slice into a version-specific ExtenderResource struct, then
// converts it to a datamodel.Extender struct and returns it, or returns an error if the unmarshal or conversion fails.
func ExtenderDataModelFromVersioned(content []byte, version string) (*datamodel.Extender, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.ExtenderResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.Extender), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
