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

// DaprInvokeHttpRouteDataModelFromVersioned converts version agnostic DaprInvokeHttpRoute datamodel to versioned model.
func DaprInvokeHttpRouteDataModelToVersioned(model *datamodel.DaprInvokeHttpRoute, version string) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.DaprInvokeHTTPRouteResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// DaprInvokeHttpRouteDataModelToVersioned converts versioned DaprInvokeHttpRoute model to datamodel.
func DaprInvokeHttpRouteDataModelFromVersioned(content []byte, version string) (*datamodel.DaprInvokeHttpRoute, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.DaprInvokeHTTPRouteResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.DaprInvokeHttpRoute), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
