package v20220315privatepreview

import (
	"errors"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/daprrp/datamodel"
	linkrpdm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// # Function Explanation
//
// ConvertTo converts a DaprStateStoreResource to a DaprStateStore and returns an error if the required
// properties are not present.
func (src *DaprStateStoreResource) ConvertTo() (v1.DataModelInterface, error) {
	daprStateStoreProperties := datamodel.DaprStateStoreProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Environment: to.String(src.Properties.GetDaprStateStoreProperties().Environment),
			Application: to.String(src.Properties.GetDaprStateStoreProperties().Application),
		},
	}

	trackedResource := v1.TrackedResource{
		ID:       to.String(src.ID),
		Name:     to.String(src.Name),
		Type:     to.String(src.Type),
		Location: to.String(src.Location),
		Tags:     to.StringMap(src.Tags),
	}
	internalMetadata := v1.InternalMetadata{
		UpdatedAPIVersion:      Version,
		AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprStateStoreProperties().ProvisioningState),
	}
	converted := &datamodel.DaprStateStore{}
	converted.TrackedResource = trackedResource
	converted.InternalMetadata = internalMetadata
	converted.Properties = daprStateStoreProperties
	switch v := src.Properties.(type) {
	case *RecipeDaprStateStoreProperties:
		if v.Recipe == nil {
			return nil, v1.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Mode = linkrpdm.LinkModeRecipe
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
	case *ResourceDaprStateStoreProperties:
		if v.Resource == nil {
			return nil, v1.NewClientErrInvalidRequest("resource is a required property for mode 'resource'")
		}
		converted.Properties.Mode = linkrpdm.LinkModeResource
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Resource = to.String(v.Resource)
	case *ValuesDaprStateStoreProperties:
		if v.Type == nil || v.Version == nil || v.Metadata == nil {
			return nil, v1.NewClientErrInvalidRequest("type/version/metadata are required properties for mode 'values'")
		}
		converted.Properties.Mode = linkrpdm.LinkModeValues
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
	default:
		return nil, errors.New("invalid mode for DaprStateStore")
	}
	return converted, nil
}

// # Function Explanation
//
// ConvertFrom converts a DataModelInterface to a DaprStateStoreResource and returns an error if the conversion fails or
// the mode of the DaprStateStore is not specified.
func (dst *DaprStateStoreResource) ConvertFrom(src v1.DataModelInterface) error {
	daprStateStore, ok := src.(*datamodel.DaprStateStore)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(daprStateStore.ID)
	dst.Name = to.Ptr(daprStateStore.Name)
	dst.Type = to.Ptr(daprStateStore.Type)
	dst.SystemData = fromSystemDataModel(daprStateStore.SystemData)
	dst.Location = to.Ptr(daprStateStore.Location)
	dst.Tags = *to.StringMapPtr(daprStateStore.Tags)

	switch daprStateStore.Properties.Mode {
	case linkrpdm.LinkModeRecipe:
		mode := "recipe"
		dst.Properties = &RecipeDaprStateStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(daprStateStore.Properties.Environment),
			Application:       to.Ptr(daprStateStore.Properties.Application),
			ComponentName:     to.Ptr(daprStateStore.Properties.ComponentName),
			Mode:              &mode,
			Recipe:            fromRecipeDataModel(daprStateStore.Properties.Recipe),
			Type:              to.Ptr(daprStateStore.Properties.Type),
			Version:           to.Ptr(daprStateStore.Properties.Version),
			Metadata:          daprStateStore.Properties.Metadata,
		}
	case linkrpdm.LinkModeResource:
		mode := "resource"
		dst.Properties = &ResourceDaprStateStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(daprStateStore.Properties.Environment),
			Application:       to.Ptr(daprStateStore.Properties.Application),
			ComponentName:     to.Ptr(daprStateStore.Properties.ComponentName),
			Mode:              &mode,
			Resource:          to.Ptr(daprStateStore.Properties.Resource),
			Metadata:          daprStateStore.Properties.Metadata,
		}
	case linkrpdm.LinkModeValues:
		mode := "values"
		dst.Properties = &ValuesDaprStateStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(daprStateStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprStateStore.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(daprStateStore.Properties.Environment),
			Application:       to.Ptr(daprStateStore.Properties.Application),
			ComponentName:     to.Ptr(daprStateStore.Properties.ComponentName),
			Mode:              &mode,
			Type:              to.Ptr(daprStateStore.Properties.Type),
			Version:           to.Ptr(daprStateStore.Properties.Version),
			Metadata:          daprStateStore.Properties.Metadata,
		}
	default:
		return errors.New("mode of DaprStateStore is not specified")
	}

	return nil
}
