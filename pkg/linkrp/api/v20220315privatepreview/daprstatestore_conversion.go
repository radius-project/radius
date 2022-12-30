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
		converted.Properties.Mode = datamodel.LinkModeRecipe
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
	case *ResourceDaprStateStoreProperties:
		if v.Resource == nil {
			return nil, conv.NewClientErrInvalidRequest("resource is a required property for mode 'resource'")
		}
		converted.Properties.Mode = datamodel.LinkModeResource
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Resource = to.String(v.Resource)
	case *ValuesDaprStateStoreProperties:
		if v.Type == nil || v.Version == nil || v.Metadata == nil {
			return nil, conv.NewClientErrInvalidRequest("type/version/metadata are required properties for mode 'values'")
		}
		converted.Properties.Mode = datamodel.LinkModeValues
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
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
	case datamodel.LinkModeRecipe:
		mode := "recipe"
		dst.Properties = &RecipeDaprStateStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprStateStore.Properties.Environment),
			Application:       to.StringPtr(daprStateStore.Properties.Application),
			ComponentName:     to.StringPtr(daprStateStore.Properties.ComponentName),
			Mode:              &mode,
			Recipe:            fromRecipeDataModel(daprStateStore.Properties.Recipe),
			Type:              to.StringPtr(daprStateStore.Properties.Type),
			Version:           to.StringPtr(daprStateStore.Properties.Version),
			Metadata:          daprStateStore.Properties.Metadata,
		}
	case datamodel.LinkModeResource:
		mode := "resource"
		dst.Properties = &ResourceDaprStateStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprStateStore.Properties.Environment),
			Application:       to.StringPtr(daprStateStore.Properties.Application),
			ComponentName:     to.StringPtr(daprStateStore.Properties.ComponentName),
			Mode:              &mode,
			Resource:          to.StringPtr(daprStateStore.Properties.Resource),
			Metadata:          daprStateStore.Properties.Metadata,
		}
	case datamodel.LinkModeValues:
		mode := "values"
		dst.Properties = &ValuesDaprStateStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprStateStore.Properties.Environment),
			Application:       to.StringPtr(daprStateStore.Properties.Application),
			ComponentName:     to.StringPtr(daprStateStore.Properties.ComponentName),
			Mode:              &mode,
			Type:              to.StringPtr(daprStateStore.Properties.Type),
			Version:           to.StringPtr(daprStateStore.Properties.Version),
			Metadata:          daprStateStore.Properties.Metadata,
		}
	default:
		return errors.New("mode of DaprStateStore is not specified")
	}

	return nil
}
