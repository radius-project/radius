// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
)

func toProvisioningStateDataModel(state *ProvisioningState) datamodel.ProvisioningStates {
	if state == nil {
		return datamodel.ProvisioningStateAccepted
	}

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
		return datamodel.ProvisioningStateAccepted
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
		converted = ProvisioningStateAccepted // should we return error ?
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
