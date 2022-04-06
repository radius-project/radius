// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315

import (
	"github.com/project-radius/radius/pkg/corerp/api"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

// ConvertTo converts from the versioned Environment resource to version-agnostic datamodel.
func (src *Environment) ConvertTo() (api.DataModelInterface, error) {
	converted := &datamodel.Environment{
		ID:         src.ID,
		Name:       src.Name,
		Type:       src.Type,
		Location:   src.Location,
		SystemData: src.SystemData,
		Properties: datamodel.EnvironmentProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Compute: datamodel.EnvironmentCompute{
				Kind:       datamodel.EnvironmentComputeKind(src.Properties.Compute.Kind),
				ResourceID: src.Properties.Compute.ResourceID,
			},
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment resource.
func (dst *Environment) ConvertFrom(src api.DataModelInterface) error {
	env := src.(*datamodel.Environment)
	dst.ID = env.ID
	dst.Name = env.Name
	dst.Type = env.Type
	dst.SystemData = env.SystemData
	dst.Location = env.Location
	dst.Properties = EnvironmentProperties{
		ProvisioningState: fromProvisioningStateDataModel(env.Properties.ProvisioningState),
		Compute: EnvironmentCompute{
			Kind:       EnvironmentComputeKind(env.Properties.Compute.Kind),
			ResourceID: env.Properties.Compute.ResourceID,
		},
	}

	return nil
}

func toProvisioningStateDataModel(state ProvisioningStates) datamodel.ProvisioningStates {
	switch state {
	case ProvisioningStateUpdating:
		return datamodel.ProvisioningStateUpdating
	case ProvisioningStateDeleting:
		return datamodel.ProvisioningStateDeleting
	case ProvisioningStateAccepted:
		return datamodel.ProvisioningStateAccepted
	case ProvisioningStateSucceeded:
		return datamodel.ProvisioningStateSucceeded
	case ProvisioningStateFailed:
		return datamodel.ProvisioningStateFailed
	case ProvisioningStateCanceled:
		return datamodel.ProvisioningStateCanceled
	default:
		return datamodel.ProvisioningStateNone
	}
}

func fromProvisioningStateDataModel(state datamodel.ProvisioningStates) ProvisioningStates {
	switch state {
	case datamodel.ProvisioningStateUpdating:
		return ProvisioningStateUpdating
	case datamodel.ProvisioningStateDeleting:
		return ProvisioningStateDeleting
	case datamodel.ProvisioningStateAccepted:
		return ProvisioningStateAccepted
	case datamodel.ProvisioningStateSucceeded:
		return ProvisioningStateSucceeded
	case datamodel.ProvisioningStateFailed:
		return ProvisioningStateFailed
	case datamodel.ProvisioningStateCanceled:
		return ProvisioningStateCanceled
	default:
		return ProvisioningStateFailed // should we return error ?
	}
}
