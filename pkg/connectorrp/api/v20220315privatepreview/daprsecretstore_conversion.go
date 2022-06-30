// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprSecretStore resource to version-agnostic datamodel.
func (src *DaprSecretStoreResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.DaprSecretStore{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.DaprSecretStoreProperties{
			ProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			Environment:       to.String(src.Properties.Environment),
			Application:       to.String(src.Properties.Application),
			Kind:              toDaprSecretStoreKindDataModel(src.Properties.Kind),
			Type:              to.String(src.Properties.Type),
			Version:           to.String(src.Properties.Version),
			Metadata:          src.Properties.Metadata,
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprSecretStore resource.
func (dst *DaprSecretStoreResource) ConvertFrom(src conv.DataModelInterface) error {
	daprSecretStore, ok := src.(*datamodel.DaprSecretStore)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(daprSecretStore.ID)
	dst.Name = to.StringPtr(daprSecretStore.Name)
	dst.Type = to.StringPtr(daprSecretStore.Type)
	dst.SystemData = fromSystemDataModel(daprSecretStore.SystemData)
	dst.Location = to.StringPtr(daprSecretStore.Location)
	dst.Tags = *to.StringMapPtr(daprSecretStore.Tags)
	dst.Properties = &DaprSecretStoreProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: v1.BuildExternalOutputResources(daprSecretStore.Properties.Status.OutputResources),
			},
		},
		ProvisioningState: fromProvisioningStateDataModel(daprSecretStore.Properties.ProvisioningState),
		Environment:       to.StringPtr(daprSecretStore.Properties.Environment),
		Application:       to.StringPtr(daprSecretStore.Properties.Application),
		Kind:              fromDaprSecretStoreKindDataModel(daprSecretStore.Properties.Kind),
		Type:              to.StringPtr(daprSecretStore.Properties.Type),
		Version:           to.StringPtr(daprSecretStore.Properties.Version),
		Metadata:          daprSecretStore.Properties.Metadata,
	}
	return nil
}

func toDaprSecretStoreKindDataModel(kind *DaprSecretStorePropertiesKind) datamodel.DaprSecretStoreKind {
	switch *kind {
	case DaprSecretStorePropertiesKindGeneric:
		return datamodel.DaprSecretStoreKindGeneric
	default:
		return datamodel.DaprSecretStoreKindUnknown
	}

}

func fromDaprSecretStoreKindDataModel(kind datamodel.DaprSecretStoreKind) *DaprSecretStorePropertiesKind {
	var convertedKind DaprSecretStorePropertiesKind
	switch kind {
	case datamodel.DaprSecretStoreKindGeneric:
		convertedKind = DaprSecretStorePropertiesKindGeneric
	default:
		convertedKind = DaprSecretStorePropertiesKindGeneric // 2022-03-15-privatprevie supports only generic.
	}
	return &convertedKind
}
