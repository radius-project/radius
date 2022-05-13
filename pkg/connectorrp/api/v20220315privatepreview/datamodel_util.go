// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/basedatamodel"
)

func toProvisioningStateDataModel(state *ProvisioningState) basedatamodel.ProvisioningStates {
	if state == nil {
		return basedatamodel.ProvisioningStateAccepted
	}

	switch *state {
	case ProvisioningStateUpdating:
		return basedatamodel.ProvisioningStateUpdating
	case ProvisioningStateDeleting:
		return basedatamodel.ProvisioningStateDeleting
	case ProvisioningStateAccepted:
		return basedatamodel.ProvisioningStateAccepted
	case ProvisioningStateSucceeded:
		return basedatamodel.ProvisioningStateSucceeded
	case ProvisioningStateFailed:
		return basedatamodel.ProvisioningStateFailed
	case ProvisioningStateCanceled:
		return basedatamodel.ProvisioningStateCanceled
	case ProvisioningStateProvisioning:
		return basedatamodel.ProvisioningStateProvisioning
	default:
		return basedatamodel.ProvisioningStateAccepted
	}
}

func fromProvisioningStateDataModel(state basedatamodel.ProvisioningStates) *ProvisioningState {
	var converted ProvisioningState
	switch state {
	case basedatamodel.ProvisioningStateUpdating:
		converted = ProvisioningStateUpdating
	case basedatamodel.ProvisioningStateDeleting:
		converted = ProvisioningStateDeleting
	case basedatamodel.ProvisioningStateAccepted:
		converted = ProvisioningStateAccepted
	case basedatamodel.ProvisioningStateSucceeded:
		converted = ProvisioningStateSucceeded
	case basedatamodel.ProvisioningStateFailed:
		converted = ProvisioningStateFailed
	case basedatamodel.ProvisioningStateCanceled:
		converted = ProvisioningStateCanceled
	default:
		converted = ProvisioningStateAccepted
	}

	return &converted
}

func unmarshalTimeString(ts string) *time.Time {
	var tt timeRFC3339
	_ = tt.UnmarshalText([]byte(ts))
	return (*time.Time)(&tt)
}

func fromSystemDataModel(s armrpcv1.SystemData) *SystemData {
	return &SystemData{
		CreatedBy:          to.StringPtr(s.CreatedBy),
		CreatedByType:      (*CreatedByType)(to.StringPtr(s.CreatedByType)),
		CreatedAt:          unmarshalTimeString(s.CreatedAt),
		LastModifiedBy:     to.StringPtr(s.LastModifiedBy),
		LastModifiedByType: (*CreatedByType)(to.StringPtr(s.LastModifiedByType)),
		LastModifiedAt:     unmarshalTimeString(s.LastModifiedAt),
	}
}
