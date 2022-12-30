// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprSecretStore resource to version-agnostic datamodel.
func (src *DaprSecretStoreResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.DaprSecretStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: Version,
			},
		},
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.GetDaprSecretStoreProperties().Environment),
				Application: to.String(src.Properties.GetDaprSecretStoreProperties().Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprSecretStoreProperties().ProvisioningState),
		},
	}
	switch v := src.Properties.(type) {
	case *ValuesDaprSecretStoreProperties:
		if v.Type == nil || v.Version == nil || v.Metadata == nil {
			return nil, conv.NewClientErrInvalidRequest("type/version/metadata are required properties for mode 'values'")
		}
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Mode = datamodel.LinkModeValues
	case *RecipeDaprSecretStoreProperties:
		if v.Recipe == nil {
			return nil, conv.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Mode = datamodel.LinkModeRecipe
	default:
		return nil, conv.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported mode %s", *src.Properties.GetDaprSecretStoreProperties().Mode))
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
	switch daprSecretStore.Properties.Mode {
	case datamodel.LinkModeValues:
		mode := "values"
		dst.Properties = &ValuesDaprSecretStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprSecretStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprSecretStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprSecretStore.Properties.Environment),
			Application:       to.StringPtr(daprSecretStore.Properties.Application),
			Mode:              &mode,
			Type:              to.StringPtr(daprSecretStore.Properties.Type),
			Version:           to.StringPtr(daprSecretStore.Properties.Version),
			Metadata:          daprSecretStore.Properties.Metadata,
			ComponentName:     to.StringPtr(daprSecretStore.Properties.ComponentName),
		}
	case datamodel.LinkModeRecipe:
		mode := "recipe"
		var recipe *Recipe
		recipe = fromRecipeDataModel(daprSecretStore.Properties.Recipe)
		dst.Properties = &RecipeDaprSecretStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprSecretStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprSecretStore.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprSecretStore.Properties.Environment),
			Application:       to.StringPtr(daprSecretStore.Properties.Application),
			Mode:              &mode,
			Type:              to.StringPtr(daprSecretStore.Properties.Type),
			Version:           to.StringPtr(daprSecretStore.Properties.Version),
			Metadata:          daprSecretStore.Properties.Metadata,
			ComponentName:     to.StringPtr(daprSecretStore.Properties.ComponentName),
			Recipe:            recipe,
		}
	}
	return nil
}
