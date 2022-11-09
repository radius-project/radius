// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned DaprPubSubBroker resource to version-agnostic datamodel.
func (src *DaprPubSubBrokerResource) ConvertTo() (conv.DataModelInterface, error) {
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
			return nil, conv.NewClientErrInvalidRequest("resource is a required property for mode 'resource'")
		}
		if *v.Kind != ResourceDaprPubSubPropertiesKindPubsubAzureServicebus {
			return nil, conv.NewClientErrInvalidRequest(fmt.Sprintf("kind must be %s when mode is 'resource'", ResourceDaprPubSubPropertiesKindPubsubAzureServicebus))
		}
		converted.Properties.Mode = datamodel.DaprPubSubBrokerModeResource
		converted.Properties.Kind = datamodel.DaprPubSubBrokerKindAzureServiceBus
		converted.Properties.Resource = to.String(v.Resource)
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
	case *ValuesDaprPubSubProperties:
		if v.Type == nil || v.Version == nil || v.Metadata == nil {
			return nil, conv.NewClientErrInvalidRequest("type/version/metadata are required properties for mode 'values'")
		}
		if *v.Kind != ValuesDaprPubSubPropertiesKindGeneric {
			return nil, conv.NewClientErrInvalidRequest(fmt.Sprintf("kind must be %s when mode is 'values'", ValuesDaprPubSubPropertiesKindGeneric))
		}
		converted.Properties.Mode = datamodel.DaprPubSubBrokerModeValues
		converted.Properties.Kind = datamodel.DaprPubSubBrokerKindGeneric
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
		converted.Properties.Resource = to.String(v.Resource)
	case *RecipeDaprPubSubProperties:
		if v.Recipe == nil {
			return nil, conv.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Mode = datamodel.DaprPubSubBrokerModeRecipe
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Resource = to.String(v.Resource)
		converted.Properties.Type = to.String(v.Type)
		converted.Properties.Version = to.String(v.Version)
		converted.Properties.Metadata = v.Metadata
	default:
		return nil, errors.New("mode of DaprPubSubBroker is not specified")
	}

	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned DaprPubSubBroker resource.
func (dst *DaprPubSubBrokerResource) ConvertFrom(src conv.DataModelInterface) error {
	daprPubSub, ok := src.(*datamodel.DaprPubSubBroker)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(daprPubSub.ID)
	dst.Name = to.StringPtr(daprPubSub.Name)
	dst.Type = to.StringPtr(daprPubSub.Type)
	dst.SystemData = fromSystemDataModel(daprPubSub.SystemData)
	dst.Location = to.StringPtr(daprPubSub.Location)
	dst.Tags = *to.StringMapPtr(daprPubSub.Tags)

	switch daprPubSub.Properties.Mode {
	case datamodel.DaprPubSubBrokerModeRecipe:
		mode := DaprPubSubBrokerPropertiesModeRecipe
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
			Resource:          to.StringPtr(daprPubSub.Properties.Resource),
			Type:              to.StringPtr(daprPubSub.Properties.Type),
			Version:           to.StringPtr(daprPubSub.Properties.Version),
			Metadata:          daprPubSub.Properties.Metadata,
			Recipe:            fromRecipeDataModel(daprPubSub.Properties.Recipe),
		}
	case datamodel.DaprPubSubBrokerModeResource:
		mode := DaprPubSubBrokerPropertiesModeResource
		kind := ResourceDaprPubSubPropertiesKindPubsubAzureServicebus
		dst.Properties = &ResourceDaprPubSubProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprPubSub.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprPubSub.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprPubSub.Properties.Environment),
			Application:       to.StringPtr(daprPubSub.Properties.Application),
			ComponentName:     to.StringPtr(daprPubSub.Properties.ComponentName),
			Mode:              &mode,
			Kind:              &kind,
			Topic:             to.StringPtr(daprPubSub.Properties.Topic),
			Resource:          to.StringPtr(daprPubSub.Properties.Resource),
			Metadata:          daprPubSub.Properties.Metadata,
		}
	case datamodel.DaprPubSubBrokerModeValues:
		mode := DaprPubSubBrokerPropertiesModeValues
		kind := ValuesDaprPubSubPropertiesKindGeneric
		dst.Properties = &ValuesDaprPubSubProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(daprPubSub.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(daprPubSub.Properties.ProvisioningState),
			Environment:       to.StringPtr(daprPubSub.Properties.Environment),
			Application:       to.StringPtr(daprPubSub.Properties.Application),
			ComponentName:     to.StringPtr(daprPubSub.Properties.ComponentName),
			Mode:              &mode,
			Kind:              &kind,
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
