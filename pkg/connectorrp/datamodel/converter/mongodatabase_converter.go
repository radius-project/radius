// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
)

// MongoDatabaseDataModelFromVersioned converts version agnostic MongoDatabase datamodel to versioned model.
func MongoDatabaseDataModelToVersioned(model conv.DataModelInterface, version string, includeSecrets bool) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		if includeSecrets {
			versioned := &v20220315privatepreview.MongoDatabaseResource{}
			err := versioned.ConvertFrom(model.(*datamodel.MongoDatabase))
			return versioned, err
		} else {
			versioned := &v20220315privatepreview.MongoDatabaseResponseResource{}
			err := versioned.ConvertFrom(model.(*datamodel.MongoDatabaseResponse))
			return versioned, err
		}
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
func MongoDatabaseSecretsDataModelToVersioned(model *datamodel.MongoDatabaseSecrets, version string) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.MongoDatabaseSecrets{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
