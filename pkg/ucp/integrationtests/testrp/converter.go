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

package testrp

import (
	"encoding/json"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
)

// TestResourceDataModelToVersioned converts version agnostic TestResource datamodel to versioned model.
func TestResourceDataModelToVersioned(model *TestResourceDatamodel, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case Version:
		versioned := &TestResource{}
		if err := versioned.ConvertFrom(model); err != nil {
			return nil, err
		}
		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// TestResourceDataModelFromVersioned converts versioned TestResource model to datamodel.
func TestResourceDataModelFromVersioned(content []byte, version string) (*TestResourceDatamodel, error) {
	switch version {
	case Version:
		am := &TestResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*TestResourceDatamodel), nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

func (src *TestResource) ConvertTo() (v1.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.

	converted := &TestResourceDatamodel{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion: Version,
				UpdatedAPIVersion: Version,
			},
		},
		Properties: TestResourceDatamodelProperties{
			Message: src.Properties.Message,
		},
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned TestResource resource.
func (dst *TestResource) ConvertFrom(src v1.DataModelInterface) error {
	tr, ok := src.(*TestResourceDatamodel)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(tr.ID)
	dst.Name = to.Ptr(tr.Name)
	dst.Type = to.Ptr(tr.Type)
	dst.Location = to.Ptr(tr.Location)
	dst.Tags = *to.StringMapPtr(tr.Tags)
	dst.Properties = TestResourceProperties{
		Message:           tr.Properties.Message,
		ProvisioningState: to.Ptr[string](string(tr.InternalMetadata.AsyncProvisioningState)),
	}

	return nil
}
