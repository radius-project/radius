/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v20231001preview

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"golang.org/x/exp/slices"
)

func toProvisioningStateDataModel(state *ProvisioningState) v1.ProvisioningState {
	if state == nil {
		return v1.ProvisioningStateAccepted
	}

	switch *state {
	case ProvisioningStateUpdating:
		return v1.ProvisioningStateUpdating
	case ProvisioningStateDeleting:
		return v1.ProvisioningStateDeleting
	case ProvisioningStateAccepted:
		return v1.ProvisioningStateAccepted
	case ProvisioningStateSucceeded:
		return v1.ProvisioningStateSucceeded
	case ProvisioningStateFailed:
		return v1.ProvisioningStateFailed
	case ProvisioningStateCanceled:
		return v1.ProvisioningStateCanceled
	default:
		return v1.ProvisioningStateAccepted
	}
}

func fromProvisioningStateDataModel(state v1.ProvisioningState) *ProvisioningState {
	var converted ProvisioningState
	switch state {
	case v1.ProvisioningStateUpdating:
		converted = ProvisioningStateUpdating
	case v1.ProvisioningStateDeleting:
		converted = ProvisioningStateDeleting
	case v1.ProvisioningStateAccepted:
		converted = ProvisioningStateAccepted
	case v1.ProvisioningStateSucceeded:
		converted = ProvisioningStateSucceeded
	case v1.ProvisioningStateFailed:
		converted = ProvisioningStateFailed
	case v1.ProvisioningStateCanceled:
		converted = ProvisioningStateCanceled
	default:
		converted = ProvisioningStateAccepted // should we return error ?
	}

	return &converted
}

func fromSystemDataModel(s v1.SystemData) *SystemData {
	return &SystemData{
		CreatedBy:          new(s.CreatedBy),
		CreatedByType:      (*CreatedByType)(new(s.CreatedByType)),
		CreatedAt:          v1.UnmarshalTimeString(s.CreatedAt),
		LastModifiedBy:     new(s.LastModifiedBy),
		LastModifiedByType: (*CreatedByType)(new(s.LastModifiedByType)),
		LastModifiedAt:     v1.UnmarshalTimeString(s.LastModifiedAt),
	}
}

func fromIdentityKind(kind rpv1.IdentitySettingKind) *IdentitySettingKind {
	switch kind {
	case rpv1.AzureIdentityWorkload:
		return new(IdentitySettingKindAzureComWorkload)
	case rpv1.SystemAssigned:
		return new(IdentitySettingKindSystemAssigned)
	case rpv1.UserAssigned:
		return new(IdentitySettingKindUserAssigned)
	case rpv1.SystemAssignedUserAssigned:
		return new(IdentitySettingKindSystemAssignedUserAssigned)
	default:
		return nil
	}
}

func toIdentityKindDataModel(kind *IdentitySettingKind) rpv1.IdentitySettingKind {
	if kind == nil {
		return rpv1.IdentityNone
	}

	switch *kind {
	case IdentitySettingKindAzureComWorkload:
		return rpv1.AzureIdentityWorkload
	case IdentitySettingKindSystemAssigned:
		return rpv1.SystemAssigned
	case IdentitySettingKindUserAssigned:
		return rpv1.UserAssigned
	case IdentitySettingKindSystemAssignedUserAssigned:
		return rpv1.SystemAssignedUserAssigned
	default:
		return rpv1.IdentityNone
	}
}

func stringSlice(s []*string) []string {
	if s == nil {
		return nil
	}
	var r []string
	for _, v := range s {
		r = append(r, *v)
	}
	return r
}

func isValidTemplateKind(templateKind string) bool {
	return slices.Contains(recipes.SupportedTemplateKind, templateKind)
}

func toOutputResourcesDataModel(outputResources []rpv1.OutputResource) []*OutputResource {
	var outResources []*OutputResource
	for _, or := range outputResources {
		r := &OutputResource{
			ID: new(or.ID.String()),
		}

		// We will not serialize the following fields if they are empty or nil.
		if or.LocalID != "" {
			r.LocalID = new(or.LocalID)
		}
		if or.RadiusManaged != nil {
			r.RadiusManaged = or.RadiusManaged
		}

		outResources = append(outResources, r)
	}
	return outResources
}
