// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
)

// RedisCacheDataModelFromVersioned converts version agnostic RedisCache datamodel to versioned model.
func RedisCacheDataModelToVersioned(model conv.DataModelInterface, version string, includeSecrets bool) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		if includeSecrets {
			versioned := &v20220315privatepreview.RedisCacheResource{}
			err := versioned.ConvertFrom(model.(*datamodel.RedisCache))
			return versioned, err
		} else {
			versioned := &v20220315privatepreview.RedisCacheResource{}
			err := versioned.ConvertFrom(model.(*datamodel.RedisCache))
			return versioned, err
		}
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

func RedisCacheSecretsDataModelToVersioned(model *datamodel.RedisCacheSecrets, version string) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.RedisCacheSecrets{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
