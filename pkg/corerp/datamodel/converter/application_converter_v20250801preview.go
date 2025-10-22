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
	v20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// Application20250801DataModelToVersioned converts version agnostic Radius.Core application datamodel to versioned model.
func Application20250801DataModelToVersioned(model *datamodel.Application_v20250801preview, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20250801preview.Version:
		versioned := &v20250801preview.ApplicationResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// Application20250801DataModelFromVersioned converts versioned Radius.Core application model to datamodel.
func Application20250801DataModelFromVersioned(content []byte, version string) (*datamodel.Application_v20250801preview, error) {
	switch version {
	case v20250801preview.Version:
		am := &v20250801preview.ApplicationResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.Application_v20250801preview), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
