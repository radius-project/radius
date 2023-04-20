// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
)

// MongoDatabaseDataModelFromVersioned converts version agnostic MongoDatabase datamodel to versioned model.
func MongoDatabaseDataModelToVersioned(model *datamodel.MongoDatabase, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.MongoDatabaseResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err
	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// MongoDatabaseDataModelToVersioned converts versioned MongoDatabase model to datamodel.
func MongoDatabaseDataModelFromVersioned(content []byte, version string) (*datamodel.MongoDatabase, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.MongoDatabaseResource{}
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
	case v20230415preview.Version:
		versioned := &v20230415preview.MongoDatabaseSecrets{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
