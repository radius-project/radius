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

// SqlDatabaseDataModelFromVersioned converts version agnostic SqlDatabase datamodel to versioned model.
func SqlDatabaseDataModelToVersioned(model *datamodel.SqlDatabase, version string) (api.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.SQLDatabaseResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
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
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}
