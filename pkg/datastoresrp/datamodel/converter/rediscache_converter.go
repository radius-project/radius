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
	"github.com/project-radius/radius/pkg/datastoresrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
)

// RedisCacheDataModelFromVersioned converts version agnostic RedisCache datamodel to versioned model.
func RedisCacheDataModelToVersioned(model *datamodel.RedisCache, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.RedisCacheResource{}
		err := versioned.ConvertFrom(model)
		if err != nil {
			return nil, err
		}

		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// RedisCacheDataModelToVersioned converts versioned RedisCache model to datamodel.
func RedisCacheDataModelFromVersioned(content []byte, version string) (*datamodel.RedisCache, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.RedisCacheResource{}
		if err := json.Unmarshal(content, versioned); err != nil {
			return nil, err
		}
		dm, err := versioned.ConvertTo()
		return dm.(*datamodel.RedisCache), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

func RedisCacheSecretsDataModelToVersioned(model *datamodel.RedisCacheSecrets, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.RedisCacheSecrets{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
