// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// HTTPRouteDataModelToVersioned converts version agnostic HTTPRoute datamodel to versioned model.
func HTTPRouteDataModelToVersioned(model *datamodel.HTTPRoute, version string) (api.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.HTTPRouteResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}

// HTTPRouteDataModelFromVersioned converts versioned HTTPRoute model to datamodel.
func HTTPRouteDataModelFromVersioned(content []byte, version string) (*datamodel.HTTPRoute, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.HTTPRouteResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.HTTPRoute), err

	default:
		return nil, basedatamodel.ErrUnsupportedAPIVersion
	}
}
