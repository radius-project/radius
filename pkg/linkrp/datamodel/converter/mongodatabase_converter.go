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

// MongoDatabaseDataModelFromVersioned converts version agnostic MongoDatabase datamodel to versioned model.
func MongoDatabaseDataModelToVersioned(model *datamodel.MongoDatabase, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.MongoDatabaseResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err
	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// MongoDatabaseDataModelToVersioned converts versioned MongoDatabase model to datamodel.
func MongoDatabaseDataModelFromVersioned(content []byte, version string) (*datamodel.MongoDatabase, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.MongoDatabaseResource{}
		if err := json.Unmarshal(content, versioned); err != nil {
			return nil, err
		}
		dm, err := versioned.ConvertTo()
		return dm.(*datamodel.MongoDatabase), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// MongoDatabaseSecretsDataModelFromVersioned converts version agnostic MongoDatabaseSecrets datamodel to versioned model.
func MongoDatabaseSecretsDataModelToVersioned(model *datamodel.MongoDatabaseSecrets, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.MongoDatabaseSecrets{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
