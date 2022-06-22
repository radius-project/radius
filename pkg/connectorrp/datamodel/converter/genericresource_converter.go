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

// GenericResourceDataModelToVersioned converts version agnostic datamodel to versioned model.
func GenericResourceDataModelToVersioned(model *datamodel.GenericResourceVersionAgnostic, version string) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.GenericResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// GenericResourceDataModelFromVersionedtaModelToVersioned converts versioned model to datamodel.
func GenericResourceDataModelFromVersioned(content []byte, version string) (*datamodel.GenericResourceVersionAgnostic, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.GenericResource{}
		if err := json.Unmarshal(content, versioned); err != nil {
			return nil, err
		}
		dm, err := versioned.ConvertTo()
		return dm.(*datamodel.GenericResourceVersionAgnostic), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
