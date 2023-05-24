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

package v20220315privatepreview

import (
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
)

// ConvertTo converts from the versioned DaprSecretStore resource to version-agnostic datamodel.
func (src *DaprSecretStoreResource) ConvertTo() (v1.DataModelInterface, error) {
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
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprSecretStoreProperties().ProvisioningState),
			},
		},
		Properties: datamodel.DaprSecretStoreProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.GetDaprSecretStoreProperties().Environment),
				Application: to.String(src.Properties.GetDaprSecretStoreProperties().Application),
			},
		},
	}
	switch v := src.Properties.(type) {
	case *ValuesDaprSecretStoreProperties:
		if v.Type == nil || v.Version == nil || v.Metadata == nil {
			return nil, v1.NewClientErrInvalidRequest("type/version/metadata are required properties for mode 'values'")
		}
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Mode = datamodel.LinkModeValues
	case *RecipeDaprSecretStoreProperties:
		if v.Recipe == nil {
			return nil, v1.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Mode = datamodel.LinkModeRecipe
	default:
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("Unsupported mode %s", *src.Properties.GetDaprSecretStoreProperties().Mode))
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprSecretStore resource.
func (dst *DaprSecretStoreResource) ConvertFrom(src v1.DataModelInterface) error {
	daprSecretStore, ok := src.(*datamodel.DaprSecretStore)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.Ptr(daprSecretStore.ID)
	dst.Name = to.Ptr(daprSecretStore.Name)
	dst.Type = to.Ptr(daprSecretStore.Type)
	dst.SystemData = fromSystemDataModel(daprSecretStore.SystemData)
	dst.Location = to.Ptr(daprSecretStore.Location)
	dst.Tags = *to.StringMapPtr(daprSecretStore.Tags)
	switch daprSecretStore.Properties.Mode {
	case datamodel.LinkModeValues:
		mode := "values"
		dst.Properties = &ValuesDaprSecretStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(daprSecretStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprSecretStore.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(daprSecretStore.Properties.Environment),
			Application:       to.Ptr(daprSecretStore.Properties.Application),
			Mode:              &mode,
			Type:              to.Ptr(daprSecretStore.Properties.Type),
			Version:           to.Ptr(daprSecretStore.Properties.Version),
			Metadata:          daprSecretStore.Properties.Metadata,
			ComponentName:     to.Ptr(daprSecretStore.Properties.ComponentName),
		}
	case datamodel.LinkModeRecipe:
		mode := "recipe"
		var recipe *Recipe
		recipe = fromRecipeDataModel(daprSecretStore.Properties.Recipe)
		dst.Properties = &RecipeDaprSecretStoreProperties{
			Status: &ResourceStatus{
				OutputResources: rpv1.BuildExternalOutputResources(daprSecretStore.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprSecretStore.InternalMetadata.AsyncProvisioningState),
			Environment:       to.Ptr(daprSecretStore.Properties.Environment),
			Application:       to.Ptr(daprSecretStore.Properties.Application),
			Mode:              &mode,
			Type:              to.Ptr(daprSecretStore.Properties.Type),
			Version:           to.Ptr(daprSecretStore.Properties.Version),
			Metadata:          daprSecretStore.Properties.Metadata,
			ComponentName:     to.Ptr(daprSecretStore.Properties.ComponentName),
			Recipe:            recipe,
		}
	}
	return nil
}
