// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/datastoresrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
)

// SqlDatabaseDataModelFromVersioned converts version agnostic SqlDatabase datamodel to versioned model.
func SqlDatabaseDataModelToVersioned(model *datamodel.SqlDatabase, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.SQLDatabaseResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// SqlDatabaseDataModelToVersioned converts versioned SqlDatabase model to datamodel.
func SqlDatabaseDataModelFromVersioned(content []byte, version string) (*datamodel.SqlDatabase, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.SQLDatabaseResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.SqlDatabase), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
