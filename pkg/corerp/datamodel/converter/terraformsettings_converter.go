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
	"bytes"
	"encoding/json"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	v20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
)

// TerraformSettingsDataModelToVersioned converts the TerraformSettings datamodel to versioned model.
func TerraformSettingsDataModelToVersioned(model *datamodel.TerraformSettings, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20250801preview.Version:
		versioned := &v20250801preview.TerraformSettingsResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// TerraformSettingsDataModelFromVersioned converts versioned TerraformSettings model to the datamodel.
func TerraformSettingsDataModelFromVersioned(content []byte, version string) (*datamodel.TerraformSettings, error) {
	switch version {
	case v20250801preview.Version:
		// Generated polymorphic decoding ignores fields from the other variant.
		// Validate the original backend first so invalid combinations and credential fields cannot disappear.
		var input struct {
			Properties struct {
				Backend json.RawMessage `json:"backend"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(content, &input); err != nil {
			return nil, err
		}
		if len(input.Properties.Backend) > 0 && !bytes.Equal(input.Properties.Backend, []byte("null")) {
			var backend datamodel.TerraformBackend
			decoder := json.NewDecoder(bytes.NewReader(input.Properties.Backend))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&backend); err != nil {
				return nil, v1.NewClientErrInvalidRequest("invalid backend: " + err.Error())
			}
			if err := backend.Validate(); err != nil {
				return nil, v1.NewClientErrInvalidRequest(err.Error())
			}
		}
		am := &v20250801preview.TerraformSettingsResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.TerraformSettings), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
