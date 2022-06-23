// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// ContainerDataModelToVersioned converts version agnostic Container datamodel to versioned model.
func ContainerDataModelToVersioned(model *datamodel.ContainerResource, version string) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.ContainerResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// ContainerDataModelFromVersioned converts versioned Container model to datamodel.
func ContainerDataModelFromVersioned(content interface{}, version string) (*datamodel.ContainerResource, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.ContainerResource{}
		if err := store.DecodeMap(content, am, v20220315privatepreview.HealthProbePropertiesClassificationHookFunc()); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.ContainerResource), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
