package v20220315privatepreview

import (
	"errors"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprStateStore resource to version-agnostic datamodel.
func (src *DaprStateStoreResource) ConvertTo() (conv.DataModelInterface, error) {
	daprStateStoreProperties := datamodel.DaprStateStoreProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Environment: to.String(src.Properties.GetDaprStateStoreProperties().Environment),
			Application: to.String(src.Properties.GetDaprStateStoreProperties().Application),
		},
		ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprStateStoreProperties().ProvisioningState),
		Kind:              toDaprStateStoreKindDataModel(src.Properties.GetDaprStateStoreProperties().Kind),
	}

	trackedResource := v1.TrackedResource{
		ID:       to.String(src.ID),
		Name:     to.String(src.Name),
		Type:     to.String(src.Type),
		Location: to.String(src.Location),
		Tags:     to.StringMap(src.Tags),
	}
	internalMetadata := v1.InternalMetadata{
		UpdatedAPIVersion: Version,
	}
	converted := &datamodel.DaprStateStore{}
	converted.TrackedResource = trackedResource
	converted.InternalMetadata = internalMetadata
	converted.Properties = daprStateStoreProperties
	switch v := src.Properties.(type) {
	case *RecipeDaprStateStoreProperties:
		if v.Recipe == nil {
			return nil, conv.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Mode = datamodel.DaprStateStoreModeRecipe
	case *ResourceDaprStateStoreResourceProperties:
		if v.Resource == nil {
			return nil, conv.NewClientErrInvalidRequest("resource is a required property for mode 'resource'")
		}
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Resource = to.String(v.Resource)
		converted.Properties.Mode = datamodel.DaprStateStoreModeResource
	case *ValuesDaprStateStoreResourceProperties:
		if v.Type == nil || v.Version == nil || v.Metadata == nil {
			return nil, conv.NewClientErrInvalidRequest("type/version/metadata are required properties for mode 'values'")
		}
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Mode = datamodel.DaprStateStoreModeResource
	default:
		return nil, errors.New("invalid mode for DaprStateStore")
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprStateStore resource.
func (dst *DaprStateStoreResource) ConvertFrom(src conv.DataModelInterface) error {
	daprStateStore, ok := src.(*datamodel.DaprStateStore)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(daprStateStore.ID)
	dst.Name = to.StringPtr(daprStateStore.Name)
	dst.Type = to.StringPtr(daprStateStore.Type)
	dst.SystemData = fromSystemDataModel(daprStateStore.SystemData)
	dst.Location = to.StringPtr(daprStateStore.Location)
	dst.Tags = *to.StringMapPtr(daprStateStore.Tags)

	switch daprStateStore.Properties.Mode {
	case datamodel.DaprStateStoreModeRecipe:
		mode := DaprStateStorePropertiesModeRecipe
		dst.Properties = &RecipeDaprStateStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprStateStore.Properties.Environment),
			Application:       to.StringPtr(daprStateStore.Properties.Application),
			Kind:              fromDaprStateStoreKindDataModel(daprStateStore.Properties.Kind),
			Mode:              &mode,
			ComponentName:     to.StringPtr(daprStateStore.Properties.ComponentName),
			Type:              to.StringPtr(daprStateStore.Properties.Type),
			Version:           to.StringPtr(daprStateStore.Properties.Version),
			Metadata:          daprStateStore.Properties.Metadata,
			Recipe:            fromRecipeDataModel(daprStateStore.Properties.Recipe),
		}
	case datamodel.DaprStateStoreModeResource:
		mode := DaprStateStorePropertiesModeResource
		dst.Properties = &ResourceDaprStateStoreResourceProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprStateStore.Properties.Environment),
			Application:       to.StringPtr(daprStateStore.Properties.Application),
			Kind:              fromDaprStateStoreKindDataModel(daprStateStore.Properties.Kind),
			Mode:              &mode,
			Resource:          to.StringPtr(daprStateStore.Properties.Resource),
			Metadata:          daprStateStore.Properties.Metadata,
			ComponentName:     to.StringPtr(daprStateStore.Properties.ComponentName),
		}
	case datamodel.DaprStateStoreModeValues:
		mode := DaprStateStorePropertiesModeValues
		dst.Properties = &ValuesDaprStateStoreResourceProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprStateStore.Properties.Environment),
			Application:       to.StringPtr(daprStateStore.Properties.Application),
			Kind:              fromDaprStateStoreKindDataModel(daprStateStore.Properties.Kind),
			Mode:              &mode,
			Type:              to.StringPtr(daprStateStore.Properties.Type),
			Version:           to.StringPtr(daprStateStore.Properties.Version),
			Metadata:          daprStateStore.Properties.Metadata,
			ComponentName:     to.StringPtr(daprStateStore.Properties.ComponentName),
		}
	default:
		return errors.New("mode of DaprStateStore is not specified")
	}

	return nil
}

func toDaprStateStoreKindDataModel(kind *DaprStateStorePropertiesKind) datamodel.DaprStateStoreKind {
	switch *kind {
	case DaprStateStorePropertiesKindStateSqlserver:
		return datamodel.DaprStateStoreKindStateSqlServer
	case DaprStateStorePropertiesKindStateAzureTablestorage:
		return datamodel.DaprStateStoreKindAzureTableStorage
	case DaprStateStorePropertiesKindGeneric:
		return datamodel.DaprStateStoreKindGeneric
	default:
		return datamodel.DaprStateStoreKindUnknown
	}

}

func fromDaprStateStoreKindDataModel(kind datamodel.DaprStateStoreKind) *DaprStateStorePropertiesKind {
	var convertedKind DaprStateStorePropertiesKind
	switch kind {
	case datamodel.DaprStateStoreKindStateSqlServer:
		convertedKind = DaprStateStorePropertiesKindStateSqlserver
	case datamodel.DaprStateStoreKindAzureTableStorage:
		convertedKind = DaprStateStorePropertiesKindStateAzureTablestorage
	case datamodel.DaprStateStoreKindGeneric:
		convertedKind = DaprStateStorePropertiesKindGeneric
	default:
		convertedKind = DaprStateStorePropertiesKindGeneric
	}
	return &convertedKind
}
