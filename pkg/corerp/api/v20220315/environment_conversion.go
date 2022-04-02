// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315

import (
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// ConvertTo converts from the versioned Environment resource to version-agnostic datamodel.
func (src *Environment) ConvertTo() (*datamodel.Environment, error) {
	converted := &datamodel.Environment{
		ID:         src.ID,
		Name:       src.Name,
		Type:       src.Type,
		Location:   src.Location,
		SystemData: src.SystemData,
		Properties: datamodel.EnvironmentProperties{
			ProvisioningState: src.Properties.ProvisioningState,
			Compute: datamodel.EnvironmentCompute{
				Kind:       datamodel.EnvironmentComputeKind(src.Properties.Compute.Kind),
				ResourceID: src.Properties.Compute.ResourceID,
			},
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment resource.
func (dst *Environment) ConvertFrom(src *datamodel.Environment) error {
	dst.ID = src.ID
	dst.Name = src.Name
	dst.Type = src.Type
	dst.SystemData = src.SystemData
	dst.Location = src.Location
	dst.Properties = EnvironmentProperties{
		ProvisioningState: src.Properties.ProvisioningState,
		Compute: EnvironmentCompute{
			Kind:       EnvironmentComputeKind(src.Properties.Compute.Kind),
			ResourceID: src.Properties.Compute.ResourceID,
		},
	}

	return nil
}
