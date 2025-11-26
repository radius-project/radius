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

package v20250801preview

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts from the versioned Environment resource to version-agnostic datamodel.
func (src *EnvironmentResource) ConvertTo() (v1.DataModelInterface, error) {
	// Note: SystemData conversion isn't required since this property comes ARM and datastore.
	converted := &datamodel.Environment_v20250801preview{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      Version,
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: datamodel.EnvironmentProperties_v20250801preview{},
	}

	// Convert RecipePacks
	if src.Properties.RecipePacks != nil {
		converted.Properties.RecipePacks = to.StringArray(src.Properties.RecipePacks)
	}

	// Convert RecipeParameters
	if src.Properties.RecipeParameters != nil {
		converted.Properties.RecipeParameters = src.Properties.RecipeParameters
	}

	// Convert Providers
	if src.Properties.Providers != nil {
		converted.Properties.Providers = toProvidersDataModel(src.Properties.Providers)
	}

	// Convert Simulated
	if src.Properties.Simulated != nil && *src.Properties.Simulated {
		converted.Properties.Simulated = true
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned Environment resource.
func (dst *EnvironmentResource) ConvertFrom(src v1.DataModelInterface) error {
	env, ok := src.(*datamodel.Environment_v20250801preview)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(env.ID)
	dst.Name = to.Ptr(env.Name)
	dst.Type = to.Ptr(env.Type)
	dst.SystemData = fromSystemDataModel(&env.SystemData)
	dst.Location = to.Ptr(env.Location)
	dst.Tags = *to.StringMapPtr(env.Tags)
	dst.Properties = &EnvironmentProperties{
		ProvisioningState: fromProvisioningStateDataModel(env.InternalMetadata.AsyncProvisioningState),
	}

	// Convert RecipePacks
	if len(env.Properties.RecipePacks) > 0 {
		dst.Properties.RecipePacks = to.ArrayofStringPtrs(env.Properties.RecipePacks)
	}

	// Convert RecipeParameters
	if len(env.Properties.RecipeParameters) > 0 {
		dst.Properties.RecipeParameters = env.Properties.RecipeParameters
	}

	// Convert Providers
	if env.Properties.Providers != nil {
		dst.Properties.Providers = fromProvidersDataModel(env.Properties.Providers)
	}

	// Convert Simulated
	if env.Properties.Simulated {
		dst.Properties.Simulated = to.Ptr(env.Properties.Simulated)
	}

	return nil
}

func toProvidersDataModel(providers *Providers) *datamodel.Providers_v20250801preview {
	if providers == nil {
		return nil
	}

	result := &datamodel.Providers_v20250801preview{}

	// Convert Azure provider
	if providers.Azure != nil {
		result.Azure = &datamodel.ProvidersAzure_v20250801preview{
			SubscriptionId:    to.String(providers.Azure.SubscriptionID),
			ResourceGroupName: to.String(providers.Azure.ResourceGroupName),
		}

		// Convert Identity
		if providers.Azure.Identity != nil {
			result.Azure.Identity = &rpv1.IdentitySettings{
				Kind:            toIdentityKindDataModel(providers.Azure.Identity.Kind),
				Resource:        to.String(providers.Azure.Identity.Resource),
				OIDCIssuer:      to.String(providers.Azure.Identity.OidcIssuer),
				ManagedIdentity: to.StringArray(providers.Azure.Identity.ManagedIdentity),
			}
		}
	}

	// Convert Kubernetes provider
	if providers.Kubernetes != nil {
		result.Kubernetes = &datamodel.ProvidersKubernetes_v20250801preview{
			Namespace: to.String(providers.Kubernetes.Namespace),
		}
	}

	// Convert AWS provider
	if providers.Aws != nil {
		result.AWS = &datamodel.ProvidersAWS_v20250801preview{
			Scope: to.String(providers.Aws.Scope),
		}
	}

	return result
}

func fromProvidersDataModel(providers *datamodel.Providers_v20250801preview) *Providers {
	if providers == nil {
		return nil
	}

	result := &Providers{}

	// Convert Azure provider
	if providers.Azure != nil {
		result.Azure = &ProvidersAzure{
			SubscriptionID:    to.Ptr(providers.Azure.SubscriptionId),
			ResourceGroupName: to.Ptr(providers.Azure.ResourceGroupName),
		}

		// Convert Identity
		if providers.Azure.Identity != nil {
			result.Azure.Identity = &IdentitySettings{
				Kind:            fromIdentityKind(providers.Azure.Identity.Kind),
				Resource:        to.Ptr(providers.Azure.Identity.Resource),
				OidcIssuer:      to.Ptr(providers.Azure.Identity.OIDCIssuer),
				ManagedIdentity: to.ArrayofStringPtrs(providers.Azure.Identity.ManagedIdentity),
			}
		}
	}

	// Convert Kubernetes provider
	if providers.Kubernetes != nil {
		result.Kubernetes = &ProvidersKubernetes{
			Namespace: to.Ptr(providers.Kubernetes.Namespace),
		}
	}

	// Convert AWS provider
	if providers.AWS != nil {
		result.Aws = &ProvidersAws{
			Scope: to.Ptr(providers.AWS.Scope),
		}
	}

	return result
}

func toProvisioningStateDataModel(state *ProvisioningState) v1.ProvisioningState {
	if state == nil {
		return v1.ProvisioningStateSucceeded
	}

	switch *state {
	case ProvisioningStateCreating:
		return v1.ProvisioningStateProvisioning
	case ProvisioningStateUpdating:
		return v1.ProvisioningStateUpdating
	case ProvisioningStateDeleting:
		return v1.ProvisioningStateDeleting
	case ProvisioningStateAccepted:
		return v1.ProvisioningStateAccepted
	case ProvisioningStateProvisioning:
		return v1.ProvisioningStateProvisioning
	case ProvisioningStateSucceeded:
		return v1.ProvisioningStateSucceeded
	case ProvisioningStateFailed:
		return v1.ProvisioningStateFailed
	case ProvisioningStateCanceled:
		return v1.ProvisioningStateCanceled
	default:
		return v1.ProvisioningStateSucceeded
	}
}

func fromProvisioningStateDataModel(state v1.ProvisioningState) *ProvisioningState {
	switch state {
	case v1.ProvisioningStateProvisioning:
		return to.Ptr(ProvisioningStateProvisioning)
	case v1.ProvisioningStateUpdating:
		return to.Ptr(ProvisioningStateUpdating)
	case v1.ProvisioningStateDeleting:
		return to.Ptr(ProvisioningStateDeleting)
	case v1.ProvisioningStateAccepted:
		return to.Ptr(ProvisioningStateAccepted)
	case v1.ProvisioningStateSucceeded:
		return to.Ptr(ProvisioningStateSucceeded)
	case v1.ProvisioningStateFailed:
		return to.Ptr(ProvisioningStateFailed)
	case v1.ProvisioningStateCanceled:
		return to.Ptr(ProvisioningStateCanceled)
	default:
		return to.Ptr(ProvisioningStateSucceeded)
	}
}

func toIdentityKindDataModel(kind *IdentitySettingKind) rpv1.IdentitySettingKind {
	if kind == nil {
		return rpv1.IdentityNone
	}

	switch *kind {
	case IdentitySettingKindUndefined:
		return rpv1.IdentityNone
	case IdentitySettingKindAzureComWorkload:
		return rpv1.AzureIdentityWorkload
	case IdentitySettingKindUserAssigned:
		return rpv1.UserAssigned
	case IdentitySettingKindSystemAssigned:
		return rpv1.SystemAssigned
	case IdentitySettingKindSystemAssignedUserAssigned:
		return rpv1.SystemAssignedUserAssigned
	default:
		return rpv1.IdentityNone
	}
}

func fromIdentityKind(kind rpv1.IdentitySettingKind) *IdentitySettingKind {
	switch kind {
	case rpv1.IdentityNone:
		return to.Ptr(IdentitySettingKindUndefined)
	case rpv1.AzureIdentityWorkload:
		return to.Ptr(IdentitySettingKindAzureComWorkload)
	case rpv1.UserAssigned:
		return to.Ptr(IdentitySettingKindUserAssigned)
	case rpv1.SystemAssigned:
		return to.Ptr(IdentitySettingKindSystemAssigned)
	case rpv1.SystemAssignedUserAssigned:
		return to.Ptr(IdentitySettingKindSystemAssignedUserAssigned)
	default:
		return to.Ptr(IdentitySettingKindUndefined)
	}
}

func fromSystemDataModel(systemData *v1.SystemData) *SystemData {
	if systemData == nil {
		return nil
	}

	return &SystemData{
		CreatedBy:          to.Ptr(systemData.CreatedBy),
		CreatedByType:      (*CreatedByType)(to.Ptr(systemData.CreatedByType)),
		CreatedAt:          v1.UnmarshalTimeString(systemData.CreatedAt),
		LastModifiedBy:     to.Ptr(systemData.LastModifiedBy),
		LastModifiedByType: (*CreatedByType)(to.Ptr(systemData.LastModifiedByType)),
		LastModifiedAt:     v1.UnmarshalTimeString(systemData.LastModifiedAt),
	}
}
