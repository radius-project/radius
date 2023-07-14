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

// # Function Explanation
//
// DaprStateStoreDataModelToVersioned converts a DaprStateStore data model to a versioned model interface
// and returns an error if the version is unsupported.
func DaprStateStoreDataModelToVersioned(model *datamodel.DaprStateStore, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.DaprStateStoreResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// # Function Explanation
//
// DaprStateStoreDataModelFromVersioned unmarshals a JSON byte slice into a DaprStateStoreResource object and converts it
// to a DaprStateStore object, returning an error if either operation fails.
func DaprStateStoreDataModelFromVersioned(content []byte, version string) (*datamodel.DaprStateStore, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.DaprStateStoreResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		if err != nil {
			return nil, err
		}

		return dm.(*datamodel.DaprStateStore), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
