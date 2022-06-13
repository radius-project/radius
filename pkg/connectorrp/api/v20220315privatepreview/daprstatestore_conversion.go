package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprStateStore resource to version-agnostic datamodel.
func (src *DaprStateStoreResource) ConvertTo() (conv.DataModelInterface, error) {
	var converted *datamodel.DaprStateStore
	daprStateStoreProperties := datamodel.DaprStateStoreProperties{
		BasicResourceProperties: v1.BasicResourceProperties{
			Status: v1.ResourceStatus{
				OutputResources: src.Properties.GetDaprStateStoreProperties().Status.OutputResources,
			},
		},
		ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprStateStoreProperties().ProvisioningState),
		Environment:       to.String(src.Properties.GetDaprStateStoreProperties().Environment),
		Application:       to.String(src.Properties.GetDaprStateStoreProperties().Application),
		Kind:              to.String(src.Properties.GetDaprStateStoreProperties().Kind),
	}
	switch v := src.Properties.(type) {
	case *DaprStateStoreAzureTableStorageResourceProperties:
		converted = &datamodel.DaprStateStore{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			Properties: &datamodel.DaprStateStoreAzureTableStorageResourceProperties{
				DaprStateStoreProperties: daprStateStoreProperties,
				Resource:                 to.String(v.Resource),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		}
	case *DaprStateStoreSQLServerResourceProperties:
		converted = &datamodel.DaprStateStore{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			Properties: &datamodel.DaprStateStoreSQLServerResourceProperties{
				DaprStateStoreProperties: daprStateStoreProperties,
				Resource:                 to.String(v.Resource),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		}
	case *DaprStateStoreGenericResourceProperties:
		converted = &datamodel.DaprStateStore{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			Properties: &datamodel.DaprStateStoreGenericResourceProperties{
				DaprStateStoreProperties: daprStateStoreProperties,
				Type:                     to.String(v.Type),
				Version:                  to.String(v.Version),
				Metadata:                 v.Metadata,
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		}
	default:
		converted = &datamodel.DaprStateStore{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			Properties: &daprStateStoreProperties,
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		}
	}
	return converted, nil
}

//ConvertFrom converts from version-agnostic datamodel to the versioned DaprStateStore resource.
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
	props := &DaprStateStoreProperties{
		BasicResourceProperties: BasicResourceProperties{
			Status: &ResourceStatus{
				OutputResources: daprStateStore.Properties.GetDaprStateStoreProperties().Status.OutputResources,
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
		dst.Properties = props
	}

	return nil
}
