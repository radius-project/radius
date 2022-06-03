package v20220315privatepreview

import (
	"errors"
	"reflect"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprStateStore resource to version-agnostic datamodel.
func (src *DaprStateStoreResource) ConvertTo() (api.DataModelInterface, error) {
	outputResources := basedatamodel.ResourceStatus{}.OutputResources
	if src.Properties.GetDaprStateStoreProperties().Status != nil {
		outputResources = src.Properties.GetDaprStateStoreProperties().Status.OutputResources
	}
	daprStateStoreProperties := datamodel.DaprStateStoreProperties{
		BasicResourceProperties: basedatamodel.BasicResourceProperties{
			Status: basedatamodel.ResourceStatus{
				OutputResources: outputResources,
			},
		},
		ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprStateStoreProperties().ProvisioningState),
		Environment:       to.String(src.Properties.GetDaprStateStoreProperties().Environment),
		Application:       to.String(src.Properties.GetDaprStateStoreProperties().Application),
		Kind:              to.String(src.Properties.GetDaprStateStoreProperties().Kind),
	}
	trackedResource := basedatamodel.TrackedResource{
		ID:       to.String(src.ID),
		Name:     to.String(src.Name),
		Type:     to.String(src.Type),
		Location: to.String(src.Location),
		Tags:     to.StringMap(src.Tags),
	}
	internalMetadata := basedatamodel.InternalMetadata{
		UpdatedAPIVersion: Version,
	}
	converted := &datamodel.DaprStateStore{}
	converted.TrackedResource = trackedResource
	converted.InternalMetadata = internalMetadata
	switch v := src.Properties.(type) {
	case *DaprStateStoreAzureTableStorageResourceProperties:
		converted.Properties = &datamodel.DaprStateStoreAzureTableStorageResourceProperties{
			DaprStateStoreProperties: daprStateStoreProperties,
			Resource:                 to.String(v.Resource),
		}
	case *DaprStateStoreSQLServerResourceProperties:
		converted.Properties = &datamodel.DaprStateStoreSQLServerResourceProperties{
			DaprStateStoreProperties: daprStateStoreProperties,
			Resource:                 to.String(v.Resource),
		}
	case *DaprStateStoreGenericResourceProperties:
		converted.Properties = &datamodel.DaprStateStoreGenericResourceProperties{
			DaprStateStoreProperties: daprStateStoreProperties,
			Type:                     to.String(v.Type),
			Version:                  to.String(v.Version),
			Metadata:                 v.Metadata,
		}
	default:
		return nil, errors.New("Kind of DaprStateStore is not specified.")
	}
	return converted, nil
}

//ConvertFrom converts from version-agnostic datamodel to the versioned DaprStateStore resource.
func (dst *DaprStateStoreResource) ConvertFrom(src api.DataModelInterface) error {
	daprStateStore, ok := src.(*datamodel.DaprStateStore)
	if !ok {
		return api.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(daprStateStore.ID)
	dst.Name = to.StringPtr(daprStateStore.Name)
	dst.Type = to.StringPtr(daprStateStore.Type)
	dst.SystemData = fromSystemDataModel(daprStateStore.SystemData)
	dst.Location = to.StringPtr(daprStateStore.Location)
	dst.Tags = *to.StringMapPtr(daprStateStore.Tags)
	var outputresources []map[string]interface{}
	if !(reflect.DeepEqual(daprStateStore.Properties.GetDaprStateStoreProperties().Status, basedatamodel.ResourceStatus{})) {
		outputresources = daprStateStore.Properties.GetDaprStateStoreProperties().Status.OutputResources
	}
	props := &DaprStateStoreProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: outputresources,
			},
		},
		ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.GetDaprStateStoreProperties().ProvisioningState),
		Environment:       to.StringPtr(daprStateStore.Properties.GetDaprStateStoreProperties().Environment),
		Application:       to.StringPtr(daprStateStore.Properties.GetDaprStateStoreProperties().Application),
	}
	switch v := daprStateStore.Properties.(type) {
	case *datamodel.DaprStateStoreAzureTableStorageResourceProperties:
		dst.Properties = &DaprStateStoreAzureTableStorageResourceProperties{
			DaprStateStoreProperties: *props,
			Resource:                 to.StringPtr(v.Resource),
		}
	case *datamodel.DaprStateStoreSQLServerResourceProperties:
		dst.Properties = &DaprStateStoreSQLServerResourceProperties{
			DaprStateStoreProperties: *props,
			Resource:                 to.StringPtr(v.Resource),
		}
	case *datamodel.DaprStateStoreGenericResourceProperties:
		dst.Properties = &DaprStateStoreGenericResourceProperties{
			DaprStateStoreProperties: *props,
			Type:                     to.StringPtr(v.Type),
			Version:                  to.StringPtr(v.Version),
			Metadata:                 v.Metadata,
		}
	default:
		return errors.New("Kind of DaprStateStore is not specified.")
	}

	return nil
}
