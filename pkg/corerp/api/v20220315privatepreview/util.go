// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
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

func unmarshalTimeString(ts string) *time.Time {
	var tt timeRFC3339
	_ = tt.UnmarshalText([]byte(ts))
	return (*time.Time)(&tt)
}

func fromSystemDataModel(s v1.SystemData) *SystemData {
	return &SystemData{
		CreatedBy:          to.Ptr(s.CreatedBy),
		CreatedByType:      (*CreatedByType)(to.Ptr(s.CreatedByType)),
		CreatedAt:          unmarshalTimeString(s.CreatedAt),
		LastModifiedBy:     to.Ptr(s.LastModifiedBy),
		LastModifiedByType: (*CreatedByType)(to.Ptr(s.LastModifiedByType)),
		LastModifiedAt:     unmarshalTimeString(s.LastModifiedAt),
	}
}

func fromIdentityKind(kind rpv1.IdentitySettingKind) *IdentitySettingKind {
	switch kind {
	case rpv1.AzureIdentityWorkload:
		return to.Ptr(IdentitySettingKindAzureComWorkload)
	default:
		return nil
	}
}

func toIdentityKind(kind *IdentitySettingKind) rpv1.IdentitySettingKind {
	if kind == nil {
		return rpv1.IdentityNone
	}

	switch *kind {
	case IdentitySettingKindAzureComWorkload:
		return rpv1.AzureIdentityWorkload
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

func isValidLinkType(link string) bool {
	linkTypes := []string{
		linkrp.DaprInvokeHttpRoutesResourceType,
		linkrp.DaprPubSubBrokersResourceType,
		linkrp.DaprSecretStoresResourceType,
		linkrp.DaprStateStoresResourceType,
		linkrp.ExtendersResourceType,
		linkrp.MongoDatabasesResourceType,
		linkrp.RabbitMQMessageQueuesResourceType,
		linkrp.RedisCachesResourceType,
		linkrp.SqlDatabasesResourceType,
	}
	return slices.Contains(linkTypes, link)
}

func isValidTemplateKind(templateKind string) bool {
	return slices.Contains(recipes.SupportedTemplateKind, templateKind)
}
