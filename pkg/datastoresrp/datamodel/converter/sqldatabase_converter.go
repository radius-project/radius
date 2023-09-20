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

// SqlDatabaseDataModelToVersioned converts a SqlDatabase data model to a VersionedModelInterface based on the specified
// version, returning an error if the version is unsupported.
func SqlDatabaseDataModelToVersioned(model *datamodel.SqlDatabase, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.SQLDatabaseResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// SqlDatabaseDataModelFromVersioned takes in a byte slice and a version string and returns a SqlDatabase object and an
// error if one occurs.
func SqlDatabaseDataModelFromVersioned(content []byte, version string) (*datamodel.SqlDatabase, error) {
	switch version {
	case v20231001preview.Version:
		am := &v20231001preview.SQLDatabaseResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		if err != nil {
			return nil, err
		}
		return dm.(*datamodel.SqlDatabase), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// This function converts a SqlDatabaseSecretsDataModel to a VersionedModelInterface based on the version provided, and
// returns an error if the version is unsupported.
func SqlDatabaseSecretsDataModelToVersioned(model *datamodel.SqlDatabaseSecrets, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.SQLDatabaseSecrets{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
