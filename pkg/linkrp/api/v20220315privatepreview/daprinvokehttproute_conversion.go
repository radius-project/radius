// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprInvokeHttpRoute resource to version-agnostic datamodel.
func (src *DaprInvokeHTTPRouteResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.DaprInvokeHttpRoute{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.DaprInvokeHttpRouteProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.GetDaprInvokeHTTPRouteProperties().Environment),
				Application: to.String(src.Properties.GetDaprInvokeHTTPRouteProperties().Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprInvokeHTTPRouteProperties().ProvisioningState),
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	switch v := src.Properties.(type) {
	case *ValuesDaprInvokeHTTPRouteProperties:
		if v.AppID == nil {
			return nil, conv.NewClientErrInvalidRequest("appId is a required property for mode 'values'")
		}
		converted.Properties.AppId = to.String(v.AppID)
		converted.Properties.Mode = datamodel.DaprInvokeHTTPRoutePropertiesModeValues
	case *RecipeDaprInvokeHTTPRouteProperties:
		if v.Recipe == nil {
			return nil, conv.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.AppId = to.String(v.AppID)
		converted.Properties.Mode = datamodel.DaprInvokeHTTPRoutePropertiesModeRecipe
	default:
		return nil, conv.NewClientErrInvalidRequest("Invalid Mode for mongo database")
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprInvokeHttpRoute resource.
func (dst *DaprInvokeHTTPRouteResource) ConvertFrom(src conv.DataModelInterface) error {
	daprHttpRoute, ok := src.(*datamodel.DaprInvokeHttpRoute)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(daprHttpRoute.ID)
	dst.Name = to.StringPtr(daprHttpRoute.Name)
	dst.Type = to.StringPtr(daprHttpRoute.Type)
	dst.SystemData = fromSystemDataModel(daprHttpRoute.SystemData)
	dst.Location = to.StringPtr(daprHttpRoute.Location)
	dst.Tags = *to.StringMapPtr(daprHttpRoute.Tags)
	switch daprHttpRoute.Properties.Mode {
	case datamodel.DaprInvokeHTTPRoutePropertiesModeValues:
		mode := DaprInvokeHTTPRoutePropertiesModeValues
		dst.Properties = &ValuesDaprInvokeHTTPRouteProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprHttpRoute.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprHttpRoute.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprHttpRoute.Properties.Environment),
			Application:       to.StringPtr(daprHttpRoute.Properties.Application),
			Mode:              &mode,
			AppID:             to.StringPtr(daprHttpRoute.Properties.AppId),
		}
	case datamodel.DaprInvokeHTTPRoutePropertiesModeRecipe:
		mode := DaprInvokeHTTPRoutePropertiesModeRecipe
		var recipe *Recipe
		if daprHttpRoute.Properties.Recipe.Name != "" {
			recipe = fromRecipeDataModel(daprHttpRoute.Properties.Recipe)
		}
		dst.Properties = &RecipeDaprInvokeHTTPRouteProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprHttpRoute.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprHttpRoute.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprHttpRoute.Properties.Environment),
			Application:       to.StringPtr(daprHttpRoute.Properties.Application),
			Mode:              &mode,
			Recipe:            recipe,
			AppID:             to.StringPtr(daprHttpRoute.Properties.AppId),
		}
	}

	return nil
}
