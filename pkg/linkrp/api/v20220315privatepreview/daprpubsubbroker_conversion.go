// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"errors"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprPubSubBroker resource to version-agnostic datamodel.
func (src *DaprPubSubBrokerResource) ConvertTo() (v1.DataModelInterface, error) {
	daprPubSubproperties := datamodel.DaprPubSubBrokerProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Environment: to.String(src.Properties.GetDaprPubSubBrokerProperties().Environment),
			Application: to.String(src.Properties.GetDaprPubSubBrokerProperties().Application),
		},
		ProvisioningState: toProvisioningStateDataModel(src.Properties.GetDaprPubSubBrokerProperties().ProvisioningState),
		Topic:             to.String(src.Properties.GetDaprPubSubBrokerProperties().Topic),
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
	converted := &datamodel.DaprPubSubBroker{}
	converted.TrackedResource = trackedResource
	converted.InternalMetadata = internalMetadata
	converted.Properties = daprPubSubproperties
	switch v := src.Properties.(type) {
	case *ResourceDaprPubSubProperties:
		if v.Resource == nil {
			return nil, v1.NewClientErrInvalidRequest("resource is a required property for mode 'resource'")
		}
		converted.Properties.Mode = datamodel.LinkModeResource
		converted.Properties.Resource = to.String(v.Resource)
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
	case *ValuesDaprPubSubProperties:
		if v.Type == nil || v.Version == nil || v.Metadata == nil {
			return nil, v1.NewClientErrInvalidRequest("type/version/metadata are required properties for mode 'values'")
		}
		converted.Properties.Mode = datamodel.LinkModeValues
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
	case *RecipeDaprPubSubProperties:
		if v.Recipe == nil {
			return nil, v1.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Mode = datamodel.LinkModeRecipe
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
	default:
		return nil, errors.New("mode of DaprPubSubBroker is not specified")
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprPubSubBroker resource.
func (dst *DaprPubSubBrokerResource) ConvertFrom(src v1.DataModelInterface) error {
	daprPubSub, ok := src.(*datamodel.DaprPubSubBroker)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(daprPubSub.ID)
	dst.Name = to.StringPtr(daprPubSub.Name)
	dst.Type = to.StringPtr(daprPubSub.Type)
	dst.SystemData = fromSystemDataModel(daprPubSub.SystemData)
	dst.Location = to.StringPtr(daprPubSub.Location)
	dst.Tags = *to.StringMapPtr(daprPubSub.Tags)

	switch daprPubSub.Properties.Mode {
	case datamodel.LinkModeRecipe:
		mode := "recipe"
		dst.Properties = &RecipeDaprPubSubProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprPubSub.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprPubSub.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprPubSub.Properties.Environment),
			Application:       to.StringPtr(daprPubSub.Properties.Application),
			ComponentName:     to.StringPtr(daprPubSub.Properties.ComponentName),
			Mode:              &mode,
			Topic:             to.StringPtr(daprPubSub.Properties.Topic),
			Type:              to.StringPtr(daprPubSub.Properties.Type),
			Version:           to.StringPtr(daprPubSub.Properties.Version),
			Metadata:          daprPubSub.Properties.Metadata,
			Recipe:            fromRecipeDataModel(daprPubSub.Properties.Recipe),
		}
	case datamodel.LinkModeResource:
		mode := "resource"
		dst.Properties = &ResourceDaprPubSubProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprPubSub.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprPubSub.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprPubSub.Properties.Environment),
			Application:       to.StringPtr(daprPubSub.Properties.Application),
			ComponentName:     to.StringPtr(daprPubSub.Properties.ComponentName),
			Mode:              &mode,
			Topic:             to.StringPtr(daprPubSub.Properties.Topic),
			Resource:          to.StringPtr(daprPubSub.Properties.Resource),
			Metadata:          daprPubSub.Properties.Metadata,
		}
	case datamodel.LinkModeValues:
		mode := "values"
		dst.Properties = &ValuesDaprPubSubProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprPubSub.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprPubSub.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprPubSub.Properties.Environment),
			Application:       to.StringPtr(daprPubSub.Properties.Application),
			ComponentName:     to.StringPtr(daprPubSub.Properties.ComponentName),
			Mode:              &mode,
			Topic:             to.StringPtr(daprPubSub.Properties.Topic),
			Type:              to.StringPtr(daprPubSub.Properties.Type),
			Version:           to.StringPtr(daprPubSub.Properties.Version),
			Metadata:          daprPubSub.Properties.Metadata,
		}
	default:
		return errors.New("mode of DaprPubSubBroker is not specified")
	}

	return nil
}
