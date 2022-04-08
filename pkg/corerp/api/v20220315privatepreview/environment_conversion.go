// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"time"

	"github.com/project-radius/radius/pkg/corerp/api"
	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned Environment resource to version-agnostic datamodel.
func (src *EnvironmentResource) ConvertTo() (api.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	converted := &datamodel.Environment{
		TrackedResource: datamodel.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.EnvironmentProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Compute: datamodel.EnvironmentCompute{
				Kind:       toEnvironmentComputeKindDataModel(src.Properties.Compute.Kind),
				ResourceID: to.String(src.Properties.Compute.ResourceID),
			},
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment resource.
func (dst *EnvironmentResource) ConvertFrom(src api.DataModelInterface) error {
	env := src.(*datamodel.Environment)
	dst.ID = to.StringPtr(env.ID)
	dst.Name = to.StringPtr(env.Name)
	dst.Type = to.StringPtr(env.Type)
	dst.SystemData = fromSystemDataModel(env.SystemData)
	dst.Location = to.StringPtr(env.Location)
	dst.Tags = *to.StringMapPtr(env.Tags)
	dst.Properties = &EnvironmentProperties{
		ProvisioningState: fromProvisioningStateDataModel(env.Properties.ProvisioningState),
		Compute: &EnvironmentCompute{
			Kind:       fromEnvironmentComputeKind(env.Properties.Compute.Kind),
			ResourceID: to.StringPtr(env.Properties.Compute.ResourceID),
		},
	}

	return nil
}

func unmarshalTimeString(ts string) *time.Time {
	var tt *timeRFC3339
	_ = tt.UnmarshalText([]byte(ts))
	return (*time.Time)(tt)
}

func fromSystemDataModel(s armrpcv1.SystemData) *SystemData {
	return &SystemData{
		CreatedBy:          to.StringPtr(s.CreatedBy),
		CreatedByType:      (*CreatedByType)(&s.CreatedByType),
		CreatedAt:          unmarshalTimeString(s.CreatedAt),
		LastModifiedBy:     to.StringPtr(s.LastModifiedBy),
		LastModifiedByType: (*CreatedByType)(&s.LastModifiedByType),
		LastModifiedAt:     unmarshalTimeString(s.LastModifiedAt),
	}
}

func toEnvironmentComputeKindDataModel(kind *EnvironmentComputeKind) datamodel.EnvironmentComputeKind {
	switch *kind {
	case EnvironmentComputeKindKubernetes:
		return datamodel.KubernetesComputeKind
	default:
		return datamodel.UnknownComputeKind
	}
}

func fromEnvironmentComputeKind(kind datamodel.EnvironmentComputeKind) *EnvironmentComputeKind {
	var k EnvironmentComputeKind
	switch kind {
	case datamodel.KubernetesComputeKind:
		k = EnvironmentComputeKindKubernetes
	}

	return &k
}

func toProvisioningStateDataModel(state *ProvisioningState) datamodel.ProvisioningStates {
	switch *state {
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

func fromProvisioningStateDataModel(state datamodel.ProvisioningStates) *ProvisioningState {
	var converted ProvisioningState
	switch state {
	case datamodel.ProvisioningStateUpdating:
		converted = ProvisioningStateUpdating
	case datamodel.ProvisioningStateDeleting:
		converted = ProvisioningStateDeleting
	case datamodel.ProvisioningStateAccepted:
		converted = ProvisioningStateAccepted
	case datamodel.ProvisioningStateSucceeded:
		converted = ProvisioningStateSucceeded
	case datamodel.ProvisioningStateFailed:
		converted = ProvisioningStateFailed
	case datamodel.ProvisioningStateCanceled:
		converted = ProvisioningStateCanceled
	default:
		converted = ProvisioningStateFailed // should we return error ?
	}

	return &converted
}
