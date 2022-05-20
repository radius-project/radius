// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
)

// RedisCacheDataModelFromVersioned converts version agnostic RedisCache datamodel to versioned model.
func RedisCacheDataModelToVersioned(model *datamodel.RedisCache, version string) (api.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.RedisCacheResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}

// RedisCacheDataModelToVersioned converts versioned RedisCache model to datamodel.
func RedisCacheDataModelFromVersioned(content []byte, version string) (*datamodel.RedisCache, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.RedisCacheResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.RedisCache), err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}
