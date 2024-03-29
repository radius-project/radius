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
	"github.com/radius-project/radius/pkg/datastoresrp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
)

// RedisCacheDataModelToVersioned converts a Redis cache data model to a versioned model interface and returns an error if
// the conversion fails.
func RedisCacheDataModelToVersioned(model *datamodel.RedisCache, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.RedisCacheResource{}
		err := versioned.ConvertFrom(model)
		if err != nil {
			return nil, err
		}

		return versioned, nil

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// RedisCacheDataModelFromVersioned converts a versioned Redis cache resource to a datamodel.RedisCache and returns an error
// if the conversion fails.
func RedisCacheDataModelFromVersioned(content []byte, version string) (*datamodel.RedisCache, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.RedisCacheResource{}
		if err := json.Unmarshal(content, versioned); err != nil {
			return nil, err
		}
		dm, err := versioned.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.RedisCache), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// RedisCacheSecretsDataModelToVersioned takes in a pointer to a RedisCacheSecrets datamodel and a version string, and
// returns a VersionedModelInterface and an error if the version is not supported.
func RedisCacheSecretsDataModelToVersioned(model *datamodel.RedisCacheSecrets, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.RedisCacheSecrets{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
