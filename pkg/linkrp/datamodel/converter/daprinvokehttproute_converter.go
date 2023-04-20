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

// DaprInvokeHttpRouteDataModelFromVersioned converts version agnostic DaprInvokeHttpRoute datamodel to versioned model.
func DaprInvokeHttpRouteDataModelToVersioned(model *datamodel.DaprInvokeHttpRoute, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20230415preview.Version:
		versioned := &v20230415preview.DaprInvokeHTTPRouteResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// DaprInvokeHttpRouteDataModelToVersioned converts versioned DaprInvokeHttpRoute model to datamodel.
func DaprInvokeHttpRouteDataModelFromVersioned(content []byte, version string) (*datamodel.DaprInvokeHttpRoute, error) {
	switch version {
	case v20230415preview.Version:
		am := &v20230415preview.DaprInvokeHTTPRouteResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.DaprInvokeHttpRoute), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
