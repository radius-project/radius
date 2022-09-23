package v20220315privatepreview

import (
	"errors"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
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

	if src.Properties.GetDaprStateStoreProperties().Recipe != nil {
		daprStateStoreProperties.Recipe = toDaprStateStoreRecipeDataModel(src.Properties.GetDaprStateStoreProperties().Recipe)
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
	case *DaprStateStoreSQLServerResourceProperties:
		converted.Properties.DaprStateStoreSQLServer = datamodel.DaprStateStoreSQLServerResourceProperties{
			Resource: to.String(v.Resource),
		}
	case *DaprStateStoreAzureTableStorageResourceProperties:
		converted.Properties.DaprStateStoreAzureTableStorage = datamodel.DaprStateStoreAzureTableStorageResourceProperties{
			Resource: to.String(v.Resource),
		}
	case *DaprStateStoreGenericResourceProperties:
		converted.Properties.DaprStateStoreGeneric = datamodel.DaprStateStoreGenericResourceProperties{
			Type:     to.String(v.Type),
			Version:  to.String(v.Version),
			Metadata: v.Metadata,
		}
	default:
		return nil, errors.New("Kind of DaprStateStore is not specified.")
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

	switch daprStateStore.Properties.Kind {
	case datamodel.DaprStateStoreKindAzureTableStorage:
		dst.Properties = &DaprStateStoreAzureTableStorageResourceProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprStateStore.Properties.Environment),
			Application:       to.StringPtr(daprStateStore.Properties.Application),
			Kind:              fromDaprStateStoreKindDataModel(daprStateStore.Properties.Kind),
			Resource:          to.StringPtr(daprStateStore.Properties.DaprStateStoreAzureTableStorage.Resource),
			ComponentName:     to.StringPtr(daprStateStore.Properties.ComponentName),
		}
	case datamodel.DaprStateStoreKindStateSqlServer:
		dst.Properties = &DaprStateStoreSQLServerResourceProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprStateStore.Properties.Environment),
			Application:       to.StringPtr(daprStateStore.Properties.Application),
			Kind:              fromDaprStateStoreKindDataModel(daprStateStore.Properties.Kind),
			Resource:          to.StringPtr(daprStateStore.Properties.DaprStateStoreSQLServer.Resource),
			ComponentName:     to.StringPtr(daprStateStore.Properties.ComponentName),
		}
	case datamodel.DaprStateStoreKindGeneric:
		dst.Properties = &DaprStateStoreGenericResourceProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprStateStore.Properties.Environment),
			Application:       to.StringPtr(daprStateStore.Properties.Application),
			Kind:              fromDaprStateStoreKindDataModel(daprStateStore.Properties.Kind),
			Type:              to.StringPtr(daprStateStore.Properties.DaprStateStoreGeneric.Type),
			Version:           to.StringPtr(daprStateStore.Properties.DaprStateStoreGeneric.Version),
			Metadata:          daprStateStore.Properties.DaprStateStoreGeneric.Metadata,
			ComponentName:     to.StringPtr(daprStateStore.Properties.ComponentName),
		}
	default:
		return errors.New("Kind of DaprStateStore is not specified.")
	}

	if daprStateStore.Properties.Recipe.Name != "" {
		dst.Properties.GetDaprStateStoreProperties().Recipe = fromDaprStateStoreRecipeDataModel(daprStateStore.Properties.Recipe)
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

func toDaprStateStoreRecipeDataModel(r *Recipe) datamodel.ConnectorRecipe {
	recipe := datamodel.ConnectorRecipe{
		Name: to.String(r.Name),
	}

	if r.Parameters != nil {
		recipe.Parameters = r.Parameters
	}
	return recipe
}

func fromDaprStateStoreRecipeDataModel(r datamodel.ConnectorRecipe) *Recipe {
	return &Recipe{
		Name:       to.StringPtr(r.Name),
		Parameters: r.Parameters,
	}
}
